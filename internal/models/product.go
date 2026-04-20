package models

import "encoding/json"

// Review represents a single product review
type Review struct {
	Rating float64 `json:"rating"`
	Review *string `json:"review,omitempty"`
}

// ProductDetails is the jsonb product_details column
type ProductDetails struct {
	Colour      string   `json:"colour"`
	Brand       string   `json:"brand"`
	Description string   `json:"description"`
	Sizes       []string `json:"sizes"`
	Reviews     []Review `json:"reviews"`
}

// ProductCategory is one row from product_categories table
type ProductCategory struct {
	CategoryID   int    `json:"category_id"`
	CategoryName string `json:"category_name"`
}

// Product is the full products table row
type Product struct {
	ProductID         int            `json:"product_id"`
	ProductName       string         `json:"product_name"`
	UnitPrice         float64        `json:"unit_price"`
	ProductDetails    ProductDetails `json:"product_details"`
	ProductCategoryID *int           `json:"product_category_id,omitempty"`
}

// SummaryResponse is the response for GET /api/products/:id/summary
type SummaryResponse struct {
	ProductID    int      `json:"product_id"`
	ProductName  string   `json:"product_name"`
	AvgRating    float64  `json:"avg_rating"`
	TotalReviews int      `json:"total_reviews"`
	Summary      string   `json:"summary"`
	Pros         []string `json:"pros"`
	Cons         []string `json:"cons"`
	Sentiment    string   `json:"sentiment"`
}

// ClaudeSummary is the JSON Claude returns for summary
type ClaudeSummary struct {
	Summary   string   `json:"summary"`
	Pros      []string `json:"pros"`
	Cons      []string `json:"cons"`
	Sentiment string   `json:"sentiment"`
}

// SearchFilters is what Claude returns for the search endpoint
type SearchFilters struct {
	Colour    *string  `json:"colour,omitempty"`
	MaxPrice  *float64 `json:"max_price,omitempty"`
	Category  *string  `json:"category,omitempty"` // category name, resolved to ID before querying
}

// RankedProduct is one entry in the ranking response
type RankedProduct struct {
	Rank        int     `json:"rank"`
	ProductID   int     `json:"product_id"`
	ProductName string  `json:"product_name"`
	Score       float64 `json:"score"`
	Reason      string  `json:"reason"`
}

// RankingResponse is the response for GET /api/products/ranking
type RankingResponse struct {
	RankedProducts []RankedProduct `json:"ranked_products"`
}

// InsightsResponse is the response for GET /api/insights
type InsightsResponse struct {
	TopPerformingBrands     []string `json:"top_performing_brands"`
	UnderperformingProducts []string `json:"underperforming_products"`
	CategoryGaps            []string `json:"category_gaps"`
	PricingRecommendations  []string `json:"pricing_recommendations"`
	GeneratedAt             string   `json:"generated_at"`
}

// MarshalProductDetails marshals ProductDetails to JSON bytes
func MarshalProductDetails(pd ProductDetails) ([]byte, error) {
	return json.Marshal(pd)
}
