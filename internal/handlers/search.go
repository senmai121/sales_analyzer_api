package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"sales_analyzer_api/internal/llm"
	"sales_analyzer_api/internal/models"
)

// SearchHandler handles GET /api/products/search?q=<query>
type SearchHandler struct {
	db     *pgxpool.Pool
	claude *llm.Client
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(db *pgxpool.Pool, claude *llm.Client) *SearchHandler {
	return &SearchHandler{db: db, claude: claude}
}

func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	// Ask Claude to extract filters from the natural language query
	prompt := fmt.Sprintf(`Extract search filters from this product search query and return a JSON object.
The JSON must have only these optional fields: colour (string), max_price (number), category (string).
Only include fields that are clearly mentioned or strongly implied in the query.
Valid category values: Electronics, Clothing, Accessories, Furniture, Kitchen, Stationery, Footwear, Home.
Query: "%s"`, query)

	responseText, err := h.claude.Complete(r.Context(), llm.JSONSystemPrompt, prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get AI filters: "+err.Error())
		return
	}

	var filters models.SearchFilters
	if err := json.Unmarshal([]byte(responseText), &filters); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse AI filter response: "+err.Error())
		return
	}

	products, err := h.searchProducts(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, products)
}

func (h *SearchHandler) searchProducts(ctx context.Context, filters models.SearchFilters) ([]models.Product, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if filters.Colour != nil && *filters.Colour != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(product_details->>'colour') = LOWER($%d)", argIdx))
		args = append(args, *filters.Colour)
		argIdx++
	}

	if filters.MaxPrice != nil {
		conditions = append(conditions, fmt.Sprintf("unit_price <= $%d", argIdx))
		args = append(args, *filters.MaxPrice)
		argIdx++
	}

	if filters.Category != nil && *filters.Category != "" {
		var categoryID int
		err := h.db.QueryRow(ctx,
			"SELECT category_id FROM product_categories WHERE LOWER(category_name) = LOWER($1)",
			*filters.Category,
		).Scan(&categoryID)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("product_category_id = $%d", argIdx))
			args = append(args, categoryID)
			argIdx++
		}
	}

	baseQuery := "SELECT product_id, product_name, unit_price, product_details, product_category_id FROM products"
	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	baseQuery += " ORDER BY product_id LIMIT 50"

	rows, err := h.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search query error: %w", err)
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		var detailsRaw []byte
		if err := rows.Scan(&p.ProductID, &p.ProductName, &p.UnitPrice, &detailsRaw, &p.ProductCategoryID); err != nil {
			return nil, fmt.Errorf("row scan error: %w", err)
		}
		if err := json.Unmarshal(detailsRaw, &p.ProductDetails); err != nil {
			return nil, fmt.Errorf("failed to parse product details: %w", err)
		}
		products = append(products, p)
	}

	if products == nil {
		products = []models.Product{}
	}

	return products, nil
}
