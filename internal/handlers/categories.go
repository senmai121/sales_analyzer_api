package handlers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"sales_analyzer_api/internal/models"
)

// CategoriesHandler handles GET /api/categories
type CategoriesHandler struct {
	db *pgxpool.Pool
}

func NewCategoriesHandler(db *pgxpool.Pool) *CategoriesHandler {
	return &CategoriesHandler{db: db}
}

func (h *CategoriesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), "SELECT category_id, category_name FROM product_categories ORDER BY category_name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch categories: "+err.Error())
		return
	}
	defer rows.Close()

	var categories []models.ProductCategory
	for rows.Next() {
		var c models.ProductCategory
		if err := rows.Scan(&c.CategoryID, &c.CategoryName); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan category: "+err.Error())
			return
		}
		categories = append(categories, c)
	}

	if categories == nil {
		categories = []models.ProductCategory{}
	}

	writeJSON(w, http.StatusOK, categories)
}
