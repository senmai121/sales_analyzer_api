package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sales_analyzer_api/internal/llm"
	"sales_analyzer_api/internal/models"
)

// SummaryHandler handles GET /api/products/:id/summary
type SummaryHandler struct {
	db     *pgxpool.Pool
	claude *llm.Client
}

// NewSummaryHandler creates a new SummaryHandler.
func NewSummaryHandler(db *pgxpool.Pool, claude *llm.Client) *SummaryHandler {
	return &SummaryHandler{db: db, claude: claude}
}

func (h *SummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := fetchProduct(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if product == nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	reviews := product.ProductDetails.Reviews
	totalReviews := len(reviews)

	// Calculate average rating
	var avgRating float64
	if totalReviews > 0 {
		var sum float64
		for _, rv := range reviews {
			sum += rv.Rating
		}
		avgRating = math.Round((sum/float64(totalReviews))*10) / 10
	}

	// Build reviews text for Claude
	reviewsJSON, _ := json.Marshal(reviews)

	prompt := fmt.Sprintf(
		"วิเคราะห์ reviews ต่อไปนี้สำหรับสินค้า %s\nสรุปเป็น JSON ที่มี: summary (2-3 ประโยค), pros (array), cons (array), sentiment (positive/neutral/negative)\nReviews: %s",
		product.ProductName,
		string(reviewsJSON),
	)

	var claudeSummary models.ClaudeSummary
	if totalReviews == 0 {
		claudeSummary = models.ClaudeSummary{
			Summary:   "ไม่มีรีวิวสำหรับสินค้านี้",
			Pros:      []string{},
			Cons:      []string{},
			Sentiment: "neutral",
		}
	} else {
		responseText, err := h.claude.Complete(r.Context(), llm.JSONSystemPrompt, prompt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get AI summary: "+err.Error())
			return
		}

		if err := json.Unmarshal([]byte(responseText), &claudeSummary); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse AI response: "+err.Error())
			return
		}
	}

	resp := models.SummaryResponse{
		ProductID:    product.ProductID,
		ProductName:  product.ProductName,
		AvgRating:    avgRating,
		TotalReviews: totalReviews,
		Summary:      claudeSummary.Summary,
		Pros:         claudeSummary.Pros,
		Cons:         claudeSummary.Cons,
		Sentiment:    claudeSummary.Sentiment,
	}

	// Ensure nil slices become empty arrays in JSON
	if resp.Pros == nil {
		resp.Pros = []string{}
	}
	if resp.Cons == nil {
		resp.Cons = []string{}
	}

	writeJSON(w, http.StatusOK, resp)
}

// fetchProduct retrieves a single product by ID. Returns nil, nil when not found.
func fetchProduct(ctx context.Context, db *pgxpool.Pool, id int) (*models.Product, error) {
	row := db.QueryRow(ctx,
		`SELECT product_id, product_name, unit_price, product_details
		 FROM products WHERE product_id = $1`, id)

	var p models.Product
	var detailsRaw []byte
	if err := row.Scan(&p.ProductID, &p.ProductName, &p.UnitPrice, &detailsRaw); err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("query error: %w", err)
	}

	if err := json.Unmarshal(detailsRaw, &p.ProductDetails); err != nil {
		return nil, fmt.Errorf("failed to parse product details: %w", err)
	}

	return &p, nil
}
