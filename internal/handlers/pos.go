package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// POSHandler handles all POS-related routes.
type POSHandler struct {
	db *pgxpool.Pool
}

// NewPOSHandler creates a new POSHandler.
func NewPOSHandler(db *pgxpool.Pool) *POSHandler {
	return &POSHandler{db: db}
}

// ---------- response types ----------

type posProductSize struct {
	Size     string `json:"size"`
	Quantity int    `json:"quantity"`
}

type posProduct struct {
	ProductID   int              `json:"product_id"`
	SKU         string           `json:"sku"`
	ProductName string           `json:"product_name"`
	UnitPrice   float64          `json:"unit_price"`
	Sizes       []posProductSize `json:"sizes"`
}

// posLocation maps to the stores table.
type posLocation struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// posCustomer maps to the customers table.
type posCustomer struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type posOrderItem struct {
	LineItemID  int     `json:"line_item_id"`
	OrderID     int     `json:"order_id"`
	ProductID   int     `json:"product_id"`
	UnitPrice   float64 `json:"unit_price"`
	Quantity    int     `json:"quantity"`
	Subtotal    float64 `json:"subtotal"`
	VatPct      float64 `json:"vat_pct"`
	VatAmount   float64 `json:"vat_amount"`
	TotalAmount float64 `json:"total_amount"`
}

type posOrder struct {
	ID          int            `json:"id"`
	StoreID     int            `json:"store_id"`
	CustomerID  int            `json:"customer_id"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	Subtotal    float64        `json:"subtotal"`
	VatAmount   float64        `json:"vat_amount"`
	TotalAmount float64        `json:"total_amount"`
	Items       []posOrderItem `json:"items,omitempty"`
}

// ---------- request body types ----------

type createCustomerRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type createOrderItemRequest struct {
	ProductID int     `json:"product_id"`
	Size      string  `json:"size"`       // kept for frontend reference only
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type createOrderRequest struct {
	StoreID    int                      `json:"store_id"`
	CustomerID int                      `json:"customer_id"` // 0 = walk-in
	Items      []createOrderItemRequest `json:"items"`
	Discount   float64                  `json:"discount"` // informational; subtracted from total
	Note       string                   `json:"note"`     // not persisted (schema doesn't have it)
}

type paymentRecord struct {
	Method    string  `json:"method"`
	Amount    float64 `json:"amount"`
	Reference string  `json:"reference"`
}

type payOrderRequest struct {
	Payments []paymentRecord `json:"payments"`
}

// ---------- walk-in customer ----------

// walkInCustomerID returns the customer_id of the reusable "Walk-in" record,
// creating it if it doesn't exist yet.
func (h *POSHandler) walkInCustomerID(r *http.Request) (int, error) {
	var id int
	err := h.db.QueryRow(r.Context(),
		"SELECT customer_id FROM customers WHERE full_name = 'Walk-in' LIMIT 1",
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	err = h.db.QueryRow(r.Context(),
		"INSERT INTO customers (full_name, email_address) VALUES ('Walk-in', 'walkin@pos.local') RETURNING customer_id",
	).Scan(&id)
	return id, err
}

// ---------- GET /api/pos/products ----------

func (h *POSHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	storeIDStr := r.URL.Query().Get("store_id")

	ctx := r.Context()

	var storeID int
	fmt.Sscanf(storeIDStr, "%d", &storeID)
	if storeID == 0 {
		if err := h.db.QueryRow(ctx,
			"SELECT store_id FROM stores ORDER BY store_id LIMIT 1",
		).Scan(&storeID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to resolve default store: "+err.Error())
			return
		}
	}

	query := `
		SELECT p.product_id, p.sku, p.product_name, p.unit_price, i.size, COALESCE(i.quantity, 0)
		FROM products p
		JOIN inventory i ON i.product_id = p.product_id
		WHERE i.store_id = $1`
	args := []interface{}{storeID}

	if q != "" {
		query += " AND (p.sku = $2 OR p.product_name ILIKE $3)"
		args = append(args, q, "%"+q+"%")
	}
	query += " ORDER BY p.product_id, i.size"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	productMap := make(map[int]*posProduct)
	var order []int

	for rows.Next() {
		var pid int
		var sku, name, size string
		var price float64
		var qty int
		if err := rows.Scan(&pid, &sku, &name, &price, &size, &qty); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		if _, ok := productMap[pid]; !ok {
			productMap[pid] = &posProduct{
				ProductID: pid, SKU: sku, ProductName: name, UnitPrice: price,
				Sizes: []posProductSize{},
			}
			order = append(order, pid)
		}
		productMap[pid].Sizes = append(productMap[pid].Sizes, posProductSize{size, qty})
	}

	result := make([]posProduct, 0, len(order))
	for _, pid := range order {
		result = append(result, *productMap[pid])
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- GET /api/pos/locations ----------

func (h *POSHandler) GetLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		"SELECT store_id, store_name, COALESCE(physical_address, '') FROM stores ORDER BY store_name",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	locations := []posLocation{}
	for rows.Next() {
		var l posLocation
		if err := rows.Scan(&l.ID, &l.Name, &l.Address); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		locations = append(locations, l)
	}
	writeJSON(w, http.StatusOK, locations)
}

// ---------- GET /api/pos/customers ----------

func (h *POSHandler) GetCustomers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	query := "SELECT customer_id, full_name, COALESCE(email_address, '') FROM customers WHERE full_name != 'Walk-in'"
	args := []interface{}{}

	if q != "" {
		query += " AND (full_name ILIKE $1 OR email_address ILIKE $1)"
		args = append(args, "%"+q+"%")
	}
	query += " ORDER BY full_name LIMIT 50"

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query error: "+err.Error())
		return
	}
	defer rows.Close()

	customers := []posCustomer{}
	for rows.Next() {
		var c posCustomer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		customers = append(customers, c)
	}
	writeJSON(w, http.StatusOK, customers)
}

// ---------- POST /api/pos/customers ----------

func (h *POSHandler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var req createCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var c posCustomer
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO customers (full_name, email_address)
		 VALUES ($1, $2)
		 RETURNING customer_id, full_name, COALESCE(email_address, '')`,
		req.Name, req.Email,
	).Scan(&c.ID, &c.Name, &c.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create customer: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ---------- POST /api/pos/orders ----------

func (h *POSHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.StoreID == 0 {
		writeError(w, http.StatusBadRequest, "store_id is required")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items must not be empty")
		return
	}

	customerID := req.CustomerID
	if customerID == 0 {
		var err error
		customerID, err = h.walkInCustomerID(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to resolve walk-in customer: "+err.Error())
			return
		}
	}

	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin transaction: "+err.Error())
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var order posOrder
	err = tx.QueryRow(ctx,
		`INSERT INTO orders (order_tms, customer_id, order_status, store_id, subtotal, vat_amount, total_amount)
		 VALUES (now(), $1, 'OPEN', $2, 0, 0, 0)
		 RETURNING order_id, store_id, customer_id, order_status, order_tms`,
		customerID, req.StoreID,
	).Scan(&order.ID, &order.StoreID, &order.CustomerID, &order.Status, &order.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to insert order: "+err.Error())
		return
	}

	const vatPct = 7.0
	order.Items = make([]posOrderItem, 0, len(req.Items))
	for _, ri := range req.Items {
		subtotal := ri.UnitPrice * float64(ri.Quantity)
		vatAmt := math.Round(subtotal*vatPct/100*100) / 100
		totalAmt := subtotal + vatAmt

		var item posOrderItem
		err = tx.QueryRow(ctx,
			`INSERT INTO order_items (order_id, product_id, unit_price, quantity, vat_pct, vat_amount, total_amount)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING line_item_id, order_id, product_id, unit_price, quantity, vat_pct, vat_amount, total_amount`,
			order.ID, ri.ProductID, ri.UnitPrice, ri.Quantity, vatPct, vatAmt, totalAmt,
		).Scan(&item.LineItemID, &item.OrderID, &item.ProductID, &item.UnitPrice, &item.Quantity,
			&item.VatPct, &item.VatAmount, &item.TotalAmount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to insert order item: "+err.Error())
			return
		}
		item.Subtotal = subtotal
		order.Subtotal += subtotal
		order.Items = append(order.Items, item)
	}
	order.VatAmount = math.Round(order.Subtotal*0.07*100) / 100
	order.TotalAmount = order.Subtotal + order.VatAmount - req.Discount

	if _, err = tx.Exec(ctx,
		`UPDATE orders SET subtotal=$1, vat_amount=$2, total_amount=$3 WHERE order_id=$4`,
		order.Subtotal, order.VatAmount, order.TotalAmount, order.ID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update order totals: "+err.Error())
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit transaction: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

// ---------- PUT /api/pos/orders/{id}/pay ----------

func (h *POSHandler) PayOrder(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "id")
	var orderID int
	if _, err := fmt.Sscanf(orderIDStr, "%d", &orderID); err != nil || orderID == 0 {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	var req payOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Payments) == 0 {
		writeError(w, http.StatusBadRequest, "payments must not be empty")
		return
	}

	ctx := r.Context()

	// Fetch order.
	var order posOrder
	err := h.db.QueryRow(ctx,
		"SELECT order_id, store_id, customer_id, order_status, order_tms FROM orders WHERE order_id = $1",
		orderID,
	).Scan(&order.ID, &order.StoreID, &order.CustomerID, &order.Status, &order.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "order not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to fetch order: "+err.Error())
		}
		return
	}
	if order.Status != "OPEN" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("order is already %s", order.Status))
		return
	}

	// Calculate total from order items.
	itemRows, err := h.db.Query(ctx,
		"SELECT unit_price, quantity FROM order_items WHERE order_id = $1",
		orderID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch items: "+err.Error())
		return
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var price float64
		var qty int
		if err := itemRows.Scan(&price, &qty); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		order.Subtotal += price * float64(qty)
	}
	order.VatAmount = math.Round(order.Subtotal*0.07*100) / 100
	order.TotalAmount = order.Subtotal + order.VatAmount

	// Validate payment sum >= total.
	var totalPaid float64
	for _, p := range req.Payments {
		totalPaid += p.Amount
	}
	if totalPaid < order.TotalAmount {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("payment total %.2f is less than order total %.2f", totalPaid, order.TotalAmount))
		return
	}

	tx, err := h.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin transaction: "+err.Error())
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, p := range req.Payments {
		var ref *string
		if p.Reference != "" {
			ref = &p.Reference
		}
		if _, err = tx.Exec(ctx,
			"INSERT INTO payments (order_id, method, amount, reference) VALUES ($1, $2, $3, $4)",
			orderID, p.Method, p.Amount, ref,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to insert payment: "+err.Error())
			return
		}
	}

	if _, err = tx.Exec(ctx,
		"UPDATE orders SET order_status = 'PAID' WHERE order_id = $1",
		orderID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update order: "+err.Error())
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit transaction: "+err.Error())
		return
	}

	order.Status = "PAID"
	writeJSON(w, http.StatusOK, order)
}
