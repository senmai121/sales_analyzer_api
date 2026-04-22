package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	llmpkg "sales_analyzer_api/internal/llm"
	"sales_analyzer_api/internal/models"
	"sales_analyzer_api/internal/sse"
)

// InsightsHandler handles GET /api/insights
type InsightsHandler struct {
	db     *pgxpool.Pool
	claude *llmpkg.Client
}

// NewInsightsHandler creates a new InsightsHandler.
func NewInsightsHandler(db *pgxpool.Pool, c *llmpkg.Client) *InsightsHandler {
	return &InsightsHandler{db: db, claude: c}
}

func (h *InsightsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	products, err := h.fetchAllProducts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	catalogJSON, err := json.Marshal(products)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal catalog: "+err.Error())
		return
	}

	cachedContext := fmt.Sprintf("Product catalog data:\n%s", string(catalogJSON))

	prompt := `Analyze the complete product catalog provided and return a JSON object with exactly these keys:
- top_performing_brands (array of strings): brands with the best overall ratings and reviews
- underperforming_products (array of strings): product names with poor ratings or minimal reviews
- category_gaps (array of strings): product categories or segments that appear to be missing or underrepresented
- pricing_recommendations (array of strings): actionable pricing suggestions based on the data`

	userPrompt := cachedContext + "\n\n" + prompt
	responseText, err := h.claude.Complete(r.Context(), llmpkg.JSONSystemPrompt, userPrompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get AI insights: "+err.Error())
		return
	}

	var insights models.InsightsResponse
	if err := json.Unmarshal([]byte(responseText), &insights); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse AI insights response: "+err.Error())
		return
	}

	insights.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	// Ensure nil slices become empty arrays
	if insights.TopPerformingBrands == nil {
		insights.TopPerformingBrands = []string{}
	}
	if insights.UnderperformingProducts == nil {
		insights.UnderperformingProducts = []string{}
	}
	if insights.CategoryGaps == nil {
		insights.CategoryGaps = []string{}
	}
	if insights.PricingRecommendations == nil {
		insights.PricingRecommendations = []string{}
	}

	writeJSON(w, http.StatusOK, insights)
}

func (h *InsightsHandler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	writer, ok := sse.New(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx := r.Context()
	ch := make(chan sse.Event, 5)

	go func() {
		defer close(ch)

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังโหลดข้อมูล catalog ทั้งหมด..."}

		products, err := h.fetchAllProducts(ctx)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: err.Error()}
			return
		}

		catalogJSON, err := json.Marshal(products)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to marshal catalog: " + err.Error()}
			return
		}

		ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังวิเคราะห์ภาพรวมด้วย AI (อาจใช้เวลาสักครู่)..."}

		cachedContext := fmt.Sprintf("Product catalog data:\n%s", string(catalogJSON))
		prompt := `Analyze the complete product catalog provided and return a JSON object with exactly these keys:
- top_performing_brands (array of strings): brands with the best overall ratings and reviews
- underperforming_products (array of strings): product names with poor ratings or minimal reviews
- category_gaps (array of strings): product categories or segments that appear to be missing or underrepresented
- pricing_recommendations (array of strings): actionable pricing suggestions based on the data`

		userPrompt := cachedContext + "\n\n" + prompt
		responseText, err := h.claude.Complete(ctx, llmpkg.JSONSystemPrompt, userPrompt)
		if err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to get AI insights: " + err.Error()}
			return
		}

		var insights models.InsightsResponse
		if err := json.Unmarshal([]byte(responseText), &insights); err != nil {
			ch <- sse.Event{Type: sse.EventError, Message: "failed to parse AI insights response: " + err.Error()}
			return
		}

		insights.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

		if insights.TopPerformingBrands == nil {
			insights.TopPerformingBrands = []string{}
		}
		if insights.UnderperformingProducts == nil {
			insights.UnderperformingProducts = []string{}
		}
		if insights.CategoryGaps == nil {
			insights.CategoryGaps = []string{}
		}
		if insights.PricingRecommendations == nil {
			insights.PricingRecommendations = []string{}
		}

		ch <- sse.Event{Type: sse.EventResult, Data: insights}
	}()

	sse.Stream(ctx, writer, ch)
}

func (h *InsightsHandler) fetchAllProducts(ctx context.Context) ([]models.Product, error) {
	rows, err := h.db.Query(ctx,
		"SELECT product_id, product_name, unit_price, product_details FROM products ORDER BY product_id")
	if err != nil {
		return nil, fmt.Errorf("insights query error: %w", err)
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		var detailsRaw []byte
		if err := rows.Scan(&p.ProductID, &p.ProductName, &p.UnitPrice, &detailsRaw); err != nil {
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
