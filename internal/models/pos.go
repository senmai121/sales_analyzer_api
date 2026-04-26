package models

import "time"

// POSStore is one row from the stores table.
type POSStore struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// POSDashboardStats is the response for GET /api/pos/dashboard/stats.
type POSDashboardStats struct {
	TodayRevenue  float64 `json:"today_revenue"`
	TodayOrders   int     `json:"today_orders"`
	AvgOrderValue float64 `json:"avg_order_value"`
	PendingOrders int     `json:"pending_orders"`
	LowStockCount int     `json:"low_stock_count"`
}

// POSRevenuePoint is one day of revenue data for the line chart.
type POSRevenuePoint struct {
	Date    string  `json:"date"`
	Revenue float64 `json:"revenue"`
	Orders  int     `json:"orders"`
}

// POSTopProduct is one entry in the top-products bar chart.
type POSTopProduct struct {
	ProductID    int     `json:"product_id"`
	ProductName  string  `json:"product_name"`
	TotalQty     int     `json:"total_qty"`
	TotalRevenue float64 `json:"total_revenue"`
}

// POSPaymentMethod is one entry in the payment-methods donut chart.
type POSPaymentMethod struct {
	Method string  `json:"method"`
	Total  float64 `json:"total"`
	Count  int     `json:"count"`
}

// POSInventoryItem is one row in the inventory response.
type POSInventoryItem struct {
	ProductID   int    `json:"product_id"`
	ProductName string `json:"product_name"`
	Size        string `json:"size"`
	Quantity    int    `json:"quantity"`
	StoreName   string `json:"store_name"`
}

// POSOrder is one row in the orders list response.
type POSOrder struct {
	ID          int       `json:"id"`
	Status      string    `json:"status"`
	TotalAmount float64   `json:"total_amount"`
	StoreID     int       `json:"store_id"`
	CreatedAt   time.Time `json:"created_at"`
	ItemsCount  int       `json:"items_count"`
}
