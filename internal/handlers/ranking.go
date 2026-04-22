package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"sales_analyzer_api/internal/llm"
	"sales_analyzer_api/internal/models"
	"sales_analyzer_api/internal/sse"
)

// RankingHandler handles GET /api/products/ranking?category=<>
type RankingHandler struct {
	db     *pgxpool.Pool
	claude *llm.Client
}

// NewRankingHandler creates a new RankingHandler.
func NewRankingHandler(db *pgxpool.Pool, claude *llm.Client) *RankingHandler {
	return &RankingHandler{db: db, claude: claude}
}

func (h *RankingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var categoryID *int
	if raw := r.URL.Query().Get("category_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "category_id must be an integer")
			return
		}
		categoryID = &id
	}

	products, err := h.fetchFilteredProducts(r.Context(), categoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(products) == 0 {
		writeJSON(w, http.StatusOK, models.RankingResponse{RankedProducts: []models.RankedProduct{}})
		return
	}

	// Build product summary for Claude
	productsJSON, err := json.Marshal(products)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal products: "+err.Error())
		return
	}

	prompt := fmt.Sprintf(`Rank the following products by their overall quality based on ratings, number of reviews, and sentiment.
Return a JSON object with key "ranked_products" containing an array of objects, each with:
- rank (integer, starting from 1)
- product_id (integer)
- product_name (string)
- score (float, 0-10)
- reason (string, brief explanation)

Products: %s`, string(productsJSON))

	responseText, err := h.claude.Complete(r.Context(), llm.JSONSystemPrompt, prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get AI ranking: "+err.Error())
		return
	}

	var rankingResp models.RankingResponse
	if err := json.Unmarshal([]byte(responseText), &rankingResp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse AI ranking response: "+err.Error())
		return
	}

	if rankingResp.RankedProducts == nil {
		rankingResp.RankedProducts = []models.RankedProduct{}
	}

	writeJSON(w, http.StatusOK, rankingResp)
}

func (h *RankingHandler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	var categoryID *int
	if raw := r.URL.Query().Get("category_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "category_id must be an integer")
			return
		}
		categoryID = &id
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

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังดึงรายการสินค้า..."}

		products, err := h.fetchFilteredProducts(ctx, categoryID)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: err.Error()}
			return
		}

		if len(products) == 0 {
			ch <- sse.Event{Type: sse.EventResult, Data: models.RankingResponse{RankedProducts: []models.RankedProduct{}}}
			return
		}

		productsJSON, err := json.Marshal(products)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to marshal products: " + err.Error()}
			return
		}

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังจัดอันดับด้วย AI..."}

		prompt := fmt.Sprintf(`Rank the following products by their overall quality based on ratings, number of reviews, and sentiment.
Return a JSON object with key "ranked_products" containing an array of objects, each with:
- rank (integer, starting from 1)
- product_id (integer)
- product_name (string)
- score (float, 0-10)
- reason (string, brief explanation)

Products: %s`, string(productsJSON))

		responseText, err := h.claude.Complete(ctx, llm.JSONSystemPrompt, prompt)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to get AI ranking: " + err.Error()}
			return
		}

		var rankingResp models.RankingResponse
		if err := json.Unmarshal([]byte(responseText), &rankingResp); err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to parse AI ranking response: " + err.Error()}
			return
		}

		if rankingResp.RankedProducts == nil {
			rankingResp.RankedProducts = []models.RankedProduct{}
		}

		ch <- sse.Event{Type: sse.EventResult, Data: rankingResp}
	}()

	sse.Stream(ctx, writer, ch)
}

func (h *RankingHandler) fetchFilteredProducts(ctx context.Context, categoryID *int) ([]models.Product, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if categoryID != nil {
		conditions = append(conditions, fmt.Sprintf("product_category_id = $%d", argIdx))
		args = append(args, *categoryID)
		argIdx++
	}

	query := "SELECT product_id, product_name, unit_price, product_details, product_category_id FROM products"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY product_id LIMIT 100"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ranking query error: %w", err)
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

	return products, nil
}
