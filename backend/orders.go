package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errEmptyCart = errors.New("cart is empty")

// errItemUnavailable marks the one per-item failure that is the client's
// problem, so the handler can answer 400 without treating every other error the
// same way. A deadlock or a dropped connection is the server's problem and must
// not be reported as a malformed request.
var errItemUnavailable = errors.New("item unavailable")

// Order statuses. An order moves forward through the kitchen queue, or leaves
// it by being cancelled.
const (
	StatusPending   = "pending"
	StatusPreparing = "preparing"
	StatusReady     = "ready"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// validTransitions is the state machine, written out rather than implied by the
// ordering of the constants. Cancelling is allowed from anywhere before the
// order is completed, because a customer can walk away up until they collect.
var validTransitions = map[string]map[string]bool{
	StatusPending:   {StatusPreparing: true, StatusCancelled: true},
	StatusPreparing: {StatusReady: true, StatusCancelled: true},
	StatusReady:     {StatusCompleted: true, StatusCancelled: true},
	StatusCompleted: {},
	StatusCancelled: {},
}

// Order is an order with its lines, as returned to clients.
type Order struct {
	ID        int64
	UserID    int64
	Customer  string
	Total     int
	Status    string
	Items     []OrderItem
	CreatedAt string
	UpdatedAt string
}

type OrderItem struct {
	MenuItemID int64
	Name       string
	Qty        int
	UnitPrice  int
	LineTotal  int
}

// placeOrder takes a cart and turns it into a paid order.
//
// Everything happens in one transaction: the order row, its lines, the student's
// debit, and the canteen's credit either all land or none do. The prices come
// from the database, never from the request, so a client cannot name its own
// price.
//
// The two ledger entries share a group id, which is what makes the transfer
// legible in the ledger: both halves reference each other and net to zero, so
// coins moved rather than being created. Only an admin exchange creates coins.
func placeOrder(db *sql.DB, userID int64, lines []CartLine) (Order, error) {
	if len(lines) == 0 {
		return Order{}, errEmptyCart
	}

	// Merge duplicate lines so the same dish is not ordered twice in one order,
	// which order_items' composite primary key would otherwise reject.
	merged := map[int64]int{}
	order := []int64{}
	for _, l := range lines {
		if l.Qty <= 0 {
			return Order{}, fmt.Errorf("%w: quantity must be positive", errInvalidAmount)
		}
		if merged[l.MenuItemID] == 0 {
			order = append(order, l.MenuItemID)
		}
		merged[l.MenuItemID] += l.Qty
	}

	tx, err := db.Begin()
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()

	// Lock the ordering user before inserting anything.
	//
	// orders.user_id is a foreign key, and a foreign key insert takes a KEY SHARE
	// lock on the referenced row. Applying the balance change afterwards needs to
	// upgrade that same lock to a full update, and two transactions doing that
	// upgrade at once deadlock each other: each waits on the other's KEY SHARE.
	// Taking the lock first, and in a fixed order, means the later FK insert
	// re-checks a lock this transaction already holds and no upgrade happens.
	var startingBalance int
	switch err := tx.QueryRow(
		`SELECT coin_balance FROM users WHERE id = $1 FOR UPDATE`, userID,
	).Scan(&startingBalance); {
	case errors.Is(err, sql.ErrNoRows):
		return Order{}, errUserNotFound
	case err != nil:
		return Order{}, fmt.Errorf("lock customer: %w", err)
	}

	canteenID, err := canteenAccountID(tx)
	if err != nil {
		return Order{}, err
	}

	items := make([]OrderItem, 0, len(order))
	total := 0
	for _, itemID := range order {
		var it OrderItem
		it.MenuItemID = itemID
		it.Qty = merged[itemID]

		// available = false is rejected here rather than filtered out, so the
		// client learns the item is sold out instead of silently getting less
		// than it asked for.
		var available bool
		err := tx.QueryRow(`
SELECT name, price::INT, available
FROM menu_items WHERE id = $1`, itemID,
		).Scan(&it.Name, &it.UnitPrice, &available)
		if errors.Is(err, sql.ErrNoRows) {
			return Order{}, fmt.Errorf("%w: menu item %d does not exist", errItemUnavailable, itemID)
		}
		if err != nil {
			return Order{}, fmt.Errorf("read menu item: %w", err)
		}
		if !available {
			return Order{}, fmt.Errorf("%w: %q is sold out", errItemUnavailable, it.Name)
		}

		it.LineTotal = it.UnitPrice * it.Qty
		total += it.LineTotal
		items = append(items, it)
	}
	if total <= 0 {
		return Order{}, errEmptyCart
	}

	var orderID int64
	if err := tx.QueryRow(`
INSERT INTO orders (user_id, total, status) VALUES ($1, $2, $3) RETURNING id`,
		userID, total, StatusPending,
	).Scan(&orderID); err != nil {
		return Order{}, fmt.Errorf("insert order: %w", err)
	}

	for _, it := range items {
		if _, err := tx.Exec(`
INSERT INTO order_items (order_id, menu_item_id, name, qty, unit_price, line_total)
VALUES ($1, $2, $3, $4, $5, $6)`,
			orderID, it.MenuItemID, it.Name, it.Qty, it.UnitPrice, it.LineTotal,
		); err != nil {
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}
	}

	group := newGroupID()
	reason := fmt.Sprintf("order #%d", orderID)
	if err := applyLedgerEntry(tx, ledgerEntry{
		UserID:  userID,
		Amount:  -total,
		Kind:    KindOrderPayment,
		Reason:  reason,
		GroupID: group,
		OrderID: &orderID,
	}); err != nil {
		return Order{}, err
	}
	if err := applyLedgerEntry(tx, ledgerEntry{
		UserID:  canteenID,
		Amount:  total,
		Kind:    KindCanteenRevenue,
		Reason:  reason,
		GroupID: group,
		OrderID: &orderID,
	}); err != nil {
		return Order{}, err
	}

	var o Order
	o.ID = orderID
	o.UserID = userID
	o.Total = total
	o.Status = StatusPending
	o.Items = items
	// Scan into any, not into the string fields: the columns are timestamptz and
	// database/sql will not convert a time.Time into a string target.
	var createdAt, updatedAt any
	if err := tx.QueryRow(`
SELECT o.created_at, o.updated_at, u.name
FROM orders o JOIN users u ON u.id = o.user_id
WHERE o.id = $1`, orderID,
	).Scan(&createdAt, &updatedAt, &o.Customer); err != nil {
		return Order{}, fmt.Errorf("read back order: %w", err)
	}
	o.CreatedAt = formatTimestamp(createdAt)
	o.UpdatedAt = formatTimestamp(updatedAt)

	if err := tx.Commit(); err != nil {
		return Order{}, err
	}
	return o, nil
}

// listOrders returns orders visible to viewer. A student or staff member sees
// only their own; the canteen and admin see the whole queue. The filter is built
// from the viewer's role, not from a parameter the client supplies, so there is
// no way to ask for someone else's orders.
func listOrders(db *sql.DB, viewer User, status string, limit int) ([]Order, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	args := []any{}
	where := []string{}
	if viewer.Role != RoleAdmin && viewer.Role != RoleCanteenManagement {
		args = append(args, viewer.ID)
		where = append(where, fmt.Sprintf("o.user_id = $%d", len(args)))
	}
	if status != "" {
		if !validStatuses[status] {
			return nil, fmt.Errorf("unknown status %q", status)
		}
		args = append(args, status)
		where = append(where, fmt.Sprintf("o.status = $%d", len(args)))
	}
	args = append(args, limit)

	q := `
SELECT o.id, o.user_id, u.name, o.total, o.status, o.created_at, o.updated_at,
       COALESCE(json_agg(json_build_object(
           'menu_item_id', oi.menu_item_id,
           'name', oi.name,
           'qty', oi.qty,
           'unit_price', oi.unit_price,
           'line_total', oi.line_total
       ) ORDER BY oi.name) FILTER (WHERE oi.order_id IS NOT NULL), '[]')
FROM orders o
JOIN users u ON u.id = o.user_id
LEFT JOIN order_items oi ON oi.order_id = o.id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += fmt.Sprintf(`
GROUP BY o.id, u.name
ORDER BY o.created_at DESC, o.id DESC
LIMIT $%d`, len(args))

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	orders := []Order{}
	for rows.Next() {
		var o Order
		var createdAt, updatedAt any
		var rawItems []byte
		if err := rows.Scan(&o.ID, &o.UserID, &o.Customer, &o.Total, &o.Status,
			&createdAt, &updatedAt, &rawItems); err != nil {
			return nil, err
		}
		if o.Items, err = decodeOrderItems(rawItems); err != nil {
			return nil, err
		}
		o.CreatedAt = formatTimestamp(createdAt)
		o.UpdatedAt = formatTimestamp(updatedAt)
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// decodeOrderItems reads the json_agg column. The driver hands it back as raw
// JSON bytes, which cannot be scanned straight into a Go slice.
func decodeOrderItems(raw []byte) ([]OrderItem, error) {
	items := []OrderItem{}
	if len(raw) == 0 {
		return items, nil
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode order items: %w", err)
	}
	return items, nil
}

var validStatuses = map[string]bool{
	StatusPending: true, StatusPreparing: true, StatusReady: true,
	StatusCompleted: true, StatusCancelled: true,
}

// setOrderStatus moves an order through the kitchen queue.
//
// Only the canteen and admin may move an order, and only along a transition the
// state machine allows. Re-cancelling a cancelled order is rejected rather than
// silently accepted, so a double tap cannot fabricate a refund path.
func setOrderStatus(db *sql.DB, orderID int64, next string) (Order, error) {
	if !validStatuses[next] {
		return Order{}, fmt.Errorf("unknown status %q", next)
	}

	tx, err := db.Begin()
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()

	var current string
	// Lock the row so two requests cannot both read 'ready' and both advance it.
	err = tx.QueryRow(
		`SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, errOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock order: %w", err)
	}

	if !validTransitions[current][next] {
		return Order{}, fmt.Errorf("cannot move an order from %s to %s", current, next)
	}

	if _, err := tx.Exec(
		`UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`, next, orderID,
	); err != nil {
		return Order{}, fmt.Errorf("update order status: %w", err)
	}

	// Cancelling refunds the customer, which is the only way coins move backwards
	// out of a completed purchase. The refund is a real ledger entry rather than
	// a deleted one, so the original payment stays visible.
	if next == StatusCancelled {
		if err := refundOrder(tx, orderID); err != nil {
			return Order{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Order{}, err
	}
	return loadOrder(db, orderID)
}

var errOrderNotFound = errors.New("order not found")

// refundOrder reverses a cancelled order's payment. The order row is not
// deleted, so the money history remains auditable.
func loadOrder(db *sql.DB, orderID int64) (Order, error) {
	var o Order
	var createdAt, updatedAt any
	var rawItems []byte
	err := db.QueryRow(`
SELECT o.id, o.user_id, u.name, o.total, o.status, o.created_at, o.updated_at,
       COALESCE((
           SELECT json_agg(json_build_object(
               'menu_item_id', oi.menu_item_id,
               'name', oi.name,
               'qty', oi.qty,
               'unit_price', oi.unit_price,
               'line_total', oi.line_total
           ) ORDER BY oi.name)
           FROM order_items oi WHERE oi.order_id = o.id
       ), '[]'::json)
FROM orders o
JOIN users u ON u.id = o.user_id
WHERE o.id = $1`, orderID,
	).Scan(&o.ID, &o.UserID, &o.Customer, &o.Total, &o.Status,
		&createdAt, &updatedAt, &rawItems)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, errOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("load order: %w", err)
	}
	if o.Items, err = decodeOrderItems(rawItems); err != nil {
		return Order{}, err
	}
	o.CreatedAt = formatTimestamp(createdAt)
	o.UpdatedAt = formatTimestamp(updatedAt)
	return o, nil
}

// refundOrder reverses a cancelled order's payment. The order row is not
// deleted, so the money history remains auditable.
func refundOrder(tx *sql.Tx, orderID int64) error {
	var userID int64
	var total int
	if err := tx.QueryRow(
		`SELECT user_id, total FROM orders WHERE id = $1`, orderID,
	).Scan(&userID, &total); err != nil {
		return fmt.Errorf("read order for refund: %w", err)
	}

	// Only a paid order can be refunded. An order cancelled before it was
	// completed still holds the student's coins, so give them back.
	var alreadyRefunded bool
	if err := tx.QueryRow(`
SELECT EXISTS (
	SELECT 1 FROM coin_transactions
	WHERE order_id = $1 AND kind = $2
)`, orderID, KindRefund).Scan(&alreadyRefunded); err != nil {
		return fmt.Errorf("check refund: %w", err)
	}
	if alreadyRefunded {
		return nil
	}

	canteenID, err := canteenAccountID(tx)
	if err != nil {
		return err
	}

	group := newGroupID()
	reason := fmt.Sprintf("refund for cancelled order #%d", orderID)
	orderRef := orderID
	if err := applyLedgerEntry(tx, ledgerEntry{
		UserID: userID, Amount: total, Kind: KindRefund,
		Reason: reason, GroupID: group, OrderID: &orderRef,
	}); err != nil {
		return err
	}
	// The canteen gives the coins back, so its balance falls by the same amount
	// it would have kept. It cannot go negative because it was credited the same
	// total when the order was placed.
	return applyLedgerEntry(tx, ledgerEntry{
		UserID: canteenID, Amount: -total, Kind: KindRefund,
		Reason: reason, GroupID: group, OrderID: &orderRef,
	})
}

// canteenAccountID finds the single canteen_management account that receives
// order revenue. Its balance is how the canteen tracks takings.
func canteenAccountID(tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow(`
SELECT id FROM users WHERE role = 'canteen_management' ORDER BY id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("no canteen_management account exists; coins have nowhere to go")
	}
	if err != nil {
		return 0, fmt.Errorf("find canteen account: %w", err)
	}
	return id, nil
}
