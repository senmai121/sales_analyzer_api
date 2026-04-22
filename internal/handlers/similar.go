package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sales_analyzer_api/internal/models"
	"sales_analyzer_api/internal/sse"
)

// SimilarHandler handles GET /api/products/:id/similar
type SimilarHandler struct {
	db *pgxpool.Pool
}

// NewSimilarHandler creates a new SimilarHandler.
func NewSimilarHandler(db *pgxpool.Pool) *SimilarHandler {
	return &SimilarHandler{db: db}
}

func (h *SimilarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	similar, err := h.findSimilarProducts(r.Context(), product)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, similar)
}

func (h *SimilarHandler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
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

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังค้นหาสินค้าที่คล้ายกัน..."}

		product, err := fetchProduct(ctx, h.db, id)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: err.Error()}
			return
		}
		if product == nil {
			ch <- sse.Event{Type: sse.EventError, Message: "product not found"}
			return
		}

		similar, err := h.findSimilarProducts(ctx, product)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: err.Error()}
			return
		}

		ch <- sse.Event{Type: sse.EventResult, Data: similar}
	}()

	sse.Stream(ctx, writer, ch)
}

func (h *SimilarHandler) findSimilarProducts(ctx context.Context, product *models.Product) ([]models.Product, error) {
	// Price range: ±30%
	minPrice := product.UnitPrice * 0.7
	maxPrice := product.UnitPrice * 1.3

	// Find products with same colour or within price range, excluding the product itself
	query := `
		SELECT product_id, product_name, unit_price, product_details,
		(
			CASE WHEN LOWER(product_details->>'colour') = LOWER($1) THEN 2 ELSE 0 END +
			CASE WHEN unit_price BETWEEN $2 AND $3 THEN 1 ELSE 0 END
		) AS similarity_score
		FROM products
		WHERE product_id != $4
		AND (
			LOWER(product_details->>'colour') = LOWER($1)
			OR unit_price BETWEEN $2 AND $3
		)
		ORDER BY similarity_score DESC, product_id
		LIMIT 10
	`

	rows, err := h.db.Query(ctx, query,
		product.ProductDetails.Colour,
		minPrice,
		maxPrice,
		product.ProductID,
	)
	if err != nil {
		return nil, fmt.Errorf("similar products query error: %w", err)
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		var detailsRaw []byte
		var score int
		if err := rows.Scan(&p.ProductID, &p.ProductName, &p.UnitPrice, &detailsRaw, &score); err != nil {
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
