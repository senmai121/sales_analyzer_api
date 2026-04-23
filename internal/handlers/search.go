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
	"sales_analyzer_api/internal/sse"
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
The JSON must have only these optional fields: keywords (array of strings), colour (string), max_price (number), category (string), min_rating (number).
- keywords: array of English synonyms for the product type (translate + include common e-commerce variants), e.g. "ย้อนยุค"→["vintage","retro","classic","antique"], "เสื้อ"→["shirt","tee","top"], "โบราณ"→["vintage","antique","retro","classic","old"]
- colour: colour in English if mentioned
- max_price: maximum price if mentioned as a number
- category: one of Electronics, Clothing, Accessories, Furniture, Kitchen, Stationery, Footwear, Home
- min_rating: minimum star rating 1-5, use when user asks for good/highly-rated/well-reviewed products (e.g. "good reviews" = 3, "great reviews" = 4)
Only include fields that are clearly mentioned or strongly implied.
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

func (h *SearchHandler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	writer, ok := sse.New(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx := r.Context()
	ch := make(chan sse.Event, 5)

	go func() {
		defer close(ch)

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังแปลงคำค้นหาด้วย AI..."}

		prompt := fmt.Sprintf(`Extract search filters from this product search query and return a JSON object.
The JSON must have only these optional fields: keywords (array of strings), colour (string), max_price (number), category (string), min_rating (number).
- keywords: array of English synonyms for the product type (translate + include common e-commerce variants), e.g. "ย้อนยุค"→["vintage","retro","classic","antique"], "เสื้อ"→["shirt","tee","top"], "โบราณ"→["vintage","antique","retro","classic","old"]
- colour: colour in English if mentioned
- max_price: maximum price if mentioned as a number
- category: one of Electronics, Clothing, Accessories, Furniture, Kitchen, Stationery, Footwear, Home
- min_rating: minimum star rating 1-5, use when user asks for good/highly-rated/well-reviewed products (e.g. "good reviews" = 3, "great reviews" = 4)
Only include fields that are clearly mentioned or strongly implied.
Query: "%s"`, query)

		responseText, err := h.claude.Complete(ctx, llm.JSONSystemPrompt, prompt)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to get AI filters: " + err.Error()}
			return
		}

		var filters models.SearchFilters
		if err := json.Unmarshal([]byte(responseText), &filters); err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to parse AI filter response: " + err.Error()}
			return
		}

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังค้นหาสินค้า..."}

		products, err := h.searchProducts(ctx, filters)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: err.Error()}
			return
		}

		ch <- sse.Event{Type: sse.EventResult, Data: products}
	}()

	sse.Stream(ctx, writer, ch)
}

func (h *SearchHandler) searchProducts(ctx context.Context, filters models.SearchFilters) ([]models.Product, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if len(filters.Keywords) > 0 {
		var kwClauses []string
		for _, kw := range filters.Keywords {
			if kw == "" {
				continue
			}
			kwClauses = append(kwClauses, fmt.Sprintf("product_name ILIKE $%d", argIdx))
			args = append(args, "%"+kw+"%")
			argIdx++
		}
		if len(kwClauses) > 0 {
			conditions = append(conditions, "("+strings.Join(kwClauses, " OR ")+")")
		}
	}

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

	if filters.MinRating != nil && *filters.MinRating > 0 {
		// ratings in DB are 0-10; min_rating from Claude is 1-5 stars, so multiply by 2
		minDB := *filters.MinRating * 2
		conditions = append(conditions, fmt.Sprintf(`(
			SELECT AVG((r->>'rating')::numeric)
			FROM jsonb_array_elements(product_details->'reviews') AS r
		) >= $%d`, argIdx))
		args = append(args, minDB)
		argIdx++
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
