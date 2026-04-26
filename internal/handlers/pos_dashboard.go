package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"sales_analyzer_api/internal/models"
)

// POSDashboardHandler handles all POS dashboard routes.
type POSDashboardHandler struct {
	db *pgxpool.Pool
}

// NewPOSDashboardHandler creates a new POSDashboardHandler.
func NewPOSDashboardHandler(db *pgxpool.Pool) *POSDashboardHandler {
	return &POSDashboardHandler{db: db}
}

// storeFilter returns an optional SQL fragment and args for filtering by store_id.
// argIdx is the 1-based index of the next placeholder.
func storeFilter(storeID string, argIdx int) (string, []interface{}) {
	if storeID == "" {
		return "", nil
	}
	id, err := strconv.Atoi(storeID)
	if err != nil {
		return "", nil
	}
	return fmt.Sprintf(" AND store_id = $%d", argIdx), []interface{}{id}
}

// ---------- GET /api/pos/locations ----------

func (h *POSDashboardHandler) GetLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT store_id, store_name, COALESCE(physical_address, '')
		 FROM stores
		 ORDER BY store_name`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	stores := []models.POSStore{}
	for rows.Next() {
		var s models.POSStore
		if err := rows.Scan(&s.ID, &s.Name, &s.Address); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		stores = append(stores, s)
	}
	writeJSON(w, http.StatusOK, stores)
}

// ---------- GET /api/pos/dashboard/stats ----------

func (h *POSDashboardHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	ctx := r.Context()

	storeFrag, storeArgs := storeFilter(storeID, 1)

	var stats models.POSDashboardStats

	// today_revenue + today_orders
	revenueQuery := fmt.Sprintf(
		`SELECT COALESCE(SUM(total_amount), 0), COUNT(*)
		 FROM orders
		 WHERE UPPER(order_status) = 'PAID'
		   AND DATE(order_tms) = CURRENT_DATE%s`,
		storeFrag,
	)
	if err := h.db.QueryRow(ctx, revenueQuery, storeArgs...).
		Scan(&stats.TodayRevenue, &stats.TodayOrders); err != nil {
		writeError(w, http.StatusInternalServerError, "stats query error: "+err.Error())
		return
	}
	if stats.TodayOrders > 0 {
		stats.AvgOrderValue = stats.TodayRevenue / float64(stats.TodayOrders)
	}

	// pending_orders
	pendingQuery := fmt.Sprintf(
		`SELECT COUNT(*) FROM orders WHERE UPPER(order_status) NOT IN ('PAID', 'CANCELLED')%s`,
		storeFrag,
	)
	if err := h.db.QueryRow(ctx, pendingQuery, storeArgs...).
		Scan(&stats.PendingOrders); err != nil {
		writeError(w, http.StatusInternalServerError, "pending query error: "+err.Error())
		return
	}

	// low_stock_count
	lowFrag, lowArgs := storeFilter(storeID, 1)
	lowStockQuery := fmt.Sprintf(
		`SELECT COUNT(*) FROM inventory WHERE quantity <= 5%s`,
		lowFrag,
	)
	if err := h.db.QueryRow(ctx, lowStockQuery, lowArgs...).
		Scan(&stats.LowStockCount); err != nil {
		writeError(w, http.StatusInternalServerError, "low stock query error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// ---------- GET /api/pos/dashboard/revenue ----------

func (h *POSDashboardHandler) GetRevenue(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	daysStr := r.URL.Query().Get("days")
	days := 7
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}
	ctx := r.Context()

	var query string
	var args []interface{}

	if storeID != "" {
		sid, _ := strconv.Atoi(storeID)
		query = `
			WITH date_series AS (
				SELECT generate_series(
					CURRENT_DATE - ($1::int - 1) * INTERVAL '1 day',
					CURRENT_DATE,
					INTERVAL '1 day'
				)::date AS day
			),
			order_agg AS (
				SELECT DATE(order_tms) AS day,
				       COALESCE(SUM(total_amount), 0) AS revenue,
				       COUNT(*) AS orders
				FROM orders
				WHERE UPPER(order_status) = 'PAID'
				  AND order_tms >= CURRENT_DATE - ($1::int - 1) * INTERVAL '1 day'
				  AND store_id = $2
				GROUP BY DATE(order_tms)
			)
			SELECT ds.day::text,
			       COALESCE(oa.revenue, 0),
			       COALESCE(oa.orders, 0)
			FROM date_series ds
			LEFT JOIN order_agg oa ON oa.day = ds.day
			ORDER BY ds.day`
		args = []interface{}{days, sid}
	} else {
		query = `
			WITH date_series AS (
				SELECT generate_series(
					CURRENT_DATE - ($1::int - 1) * INTERVAL '1 day',
					CURRENT_DATE,
					INTERVAL '1 day'
				)::date AS day
			),
			order_agg AS (
				SELECT DATE(order_tms) AS day,
				       COALESCE(SUM(total_amount), 0) AS revenue,
				       COUNT(*) AS orders
				FROM orders
				WHERE UPPER(order_status) = 'PAID'
				  AND order_tms >= CURRENT_DATE - ($1::int - 1) * INTERVAL '1 day'
				GROUP BY DATE(order_tms)
			)
			SELECT ds.day::text,
			       COALESCE(oa.revenue, 0),
			       COALESCE(oa.orders, 0)
			FROM date_series ds
			LEFT JOIN order_agg oa ON oa.day = ds.day
			ORDER BY ds.day`
		args = []interface{}{days}
	}

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	result := []models.POSRevenuePoint{}
	for rows.Next() {
		var p models.POSRevenuePoint
		if err := rows.Scan(&p.Date, &p.Revenue, &p.Orders); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		result = append(result, p)
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- GET /api/pos/dashboard/top-products ----------

func (h *POSDashboardHandler) GetTopProducts(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	ctx := r.Context()

	var query string
	var args []interface{}

	if storeID != "" {
		sid, _ := strconv.Atoi(storeID)
		query = `
			SELECT oi.product_id,
			       p.product_name,
			       SUM(oi.quantity)::int              AS total_qty,
			       COALESCE(SUM(oi.total_amount), 0) AS total_revenue
			FROM order_items oi
			JOIN orders  o ON o.order_id  = oi.order_id
			JOIN products p ON p.product_id = oi.product_id
			WHERE UPPER(o.order_status) = 'PAID'
			  AND o.store_id = $1
			GROUP BY oi.product_id, p.product_name
			ORDER BY total_qty DESC
			LIMIT $2`
		args = []interface{}{sid, limit}
	} else {
		query = `
			SELECT oi.product_id,
			       p.product_name,
			       SUM(oi.quantity)::int              AS total_qty,
			       COALESCE(SUM(oi.total_amount), 0) AS total_revenue
			FROM order_items oi
			JOIN orders  o ON o.order_id  = oi.order_id
			JOIN products p ON p.product_id = oi.product_id
			WHERE UPPER(o.order_status) = 'PAID'
			GROUP BY oi.product_id, p.product_name
			ORDER BY total_qty DESC
			LIMIT $1`
		args = []interface{}{limit}
	}

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	result := []models.POSTopProduct{}
	for rows.Next() {
		var p models.POSTopProduct
		if err := rows.Scan(&p.ProductID, &p.ProductName, &p.TotalQty, &p.TotalRevenue); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		result = append(result, p)
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- GET /api/pos/dashboard/payment-methods ----------

func (h *POSDashboardHandler) GetPaymentMethods(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	ctx := r.Context()

	var query string
	var args []interface{}

	if storeID != "" {
		sid, _ := strconv.Atoi(storeID)
		query = `
			SELECT py.method,
			       COALESCE(SUM(py.amount), 0) AS total,
			       COUNT(*)::int               AS count
			FROM payments py
			JOIN orders o ON o.order_id = py.order_id
			WHERE UPPER(o.order_status) = 'PAID'
			  AND o.store_id = $1
			GROUP BY py.method
			ORDER BY total DESC`
		args = []interface{}{sid}
	} else {
		query = `
			SELECT py.method,
			       COALESCE(SUM(py.amount), 0) AS total,
			       COUNT(*)::int               AS count
			FROM payments py
			JOIN orders o ON o.order_id = py.order_id
			WHERE UPPER(o.order_status) = 'PAID'
			GROUP BY py.method
			ORDER BY total DESC`
	}

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	result := []models.POSPaymentMethod{}
	for rows.Next() {
		var m models.POSPaymentMethod
		if err := rows.Scan(&m.Method, &m.Total, &m.Count); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		result = append(result, m)
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- GET /api/pos/inventory ----------

func (h *POSDashboardHandler) GetInventory(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	ctx := r.Context()

	var query string
	var args []interface{}

	if storeID != "" {
		sid, _ := strconv.Atoi(storeID)
		query = `
			SELECT p.product_id,
			       p.product_name,
			       i.size,
			       i.quantity,
			       s.store_name
			FROM inventory i
			JOIN products p ON p.product_id = i.product_id
			JOIN stores   s ON s.store_id   = i.store_id
			WHERE i.store_id = $1
			ORDER BY i.quantity ASC, p.product_name`
		args = []interface{}{sid}
	} else {
		query = `
			SELECT p.product_id,
			       p.product_name,
			       i.size,
			       i.quantity,
			       s.store_name
			FROM inventory i
			JOIN products p ON p.product_id = i.product_id
			JOIN stores   s ON s.store_id   = i.store_id
			ORDER BY i.quantity ASC, p.product_name`
	}

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	result := []models.POSInventoryItem{}
	for rows.Next() {
		var item models.POSInventoryItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.Size, &item.Quantity, &item.StoreName); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- GET /api/pos/orders ----------

func (h *POSDashboardHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if off, err := strconv.Atoi(offsetStr); err == nil && off >= 0 {
			offset = off
		}
	}

	ctx := r.Context()

	where := "WHERE 1=1"
	args := []interface{}{}
	idx := 1

	if storeID != "" {
		sid, _ := strconv.Atoi(storeID)
		where += fmt.Sprintf(" AND o.store_id = $%d", idx)
		args = append(args, sid)
		idx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND UPPER(o.order_status) = UPPER($%d)", idx)
		args = append(args, status)
		idx++
	}

	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT o.order_id,
		       o.order_status,
		       COALESCE(o.total_amount, 0),
		       o.store_id,
		       o.order_tms,
		       COUNT(oi.line_item_id)::int AS items_count
		FROM orders o
		LEFT JOIN order_items oi ON oi.order_id = o.order_id
		%s
		GROUP BY o.order_id, o.order_status, o.total_amount, o.store_id, o.order_tms
		ORDER BY o.order_tms DESC
		LIMIT $%d OFFSET $%d`,
		where, idx, idx+1,
	)

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	result := []models.POSOrder{}
	for rows.Next() {
		var o models.POSOrder
		if err := rows.Scan(&o.ID, &o.Status, &o.TotalAmount, &o.StoreID,
			&o.CreatedAt, &o.ItemsCount); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		result = append(result, o)
	}
	writeJSON(w, http.StatusOK, result)
}
