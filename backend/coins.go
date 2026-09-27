package main

import (
	"database/sql"
	"errors"
	"fmt"
)

var errInsufficientCoins = errors.New("insufficient coin balance")
var errInvalidAmount = errors.New("amount must be a positive whole number of coins")

// Ledger kinds. Each one answers a different question about the economy:
// exchange_in is cash the admin took for coins, order_payment and
// canteen_revenue are the two halves of a purchase, and refund returns coins.
const (
	KindExchangeIn     = "exchange_in"
	KindOrderPayment   = "order_payment"
	KindCanteenRevenue = "canteen_revenue"
	KindRefund         = "refund"
)

// ledgerEntry is one balance change. Amount is signed: positive credits the
// user, negative debits them.
type ledgerEntry struct {
	UserID  int64
	Amount  int
	Kind    string
	Reason  string
	ActorID *int64 // the admin who initiated it, nil for system entries
	OrderID *int64 // set for order settlements
	GroupID string // shared by both halves of a transfer; empty means mint
}

// applyLedgerEntry is the single place in the codebase where a coin balance
// changes. Every caller goes through it, which is what keeps the cached
// coin_balance and the ledger from drifting: both are written here, from the
// same inputs, in the caller's transaction.
//
// It must be called inside a transaction. The row lock is taken before the
// balance is read, so two orders spending the same coins serialise instead of
// both passing the balance check.
func applyLedgerEntry(tx *sql.Tx, e ledgerEntry) error {
	if e.Amount == 0 {
		return errInvalidAmount
	}

	var balance int
	err := tx.QueryRow(
		`SELECT coin_balance FROM users WHERE id = $1 FOR UPDATE`, e.UserID,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return errUserNotFound
	}
	if err != nil {
		return fmt.Errorf("lock balance: %w", err)
	}

	next := balance + e.Amount
	if next < 0 {
		// The users_coin_nonneg constraint would also catch this, but failing
		// here produces a useful message instead of a constraint violation.
		return fmt.Errorf("%w: has %d, needs %d", errInsufficientCoins, balance, -e.Amount)
	}

	if _, err := tx.Exec(
		`UPDATE users SET coin_balance = $1 WHERE id = $2`, next, e.UserID,
	); err != nil {
		return fmt.Errorf("update balance: %w", err)
	}

	if _, err := tx.Exec(`
INSERT INTO coin_transactions (group_id, user_id, amount, kind, reason, actor_id, order_id)
VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7)`,
		e.GroupID, e.UserID, e.Amount, e.Kind, e.Reason, e.ActorID, e.OrderID,
	); err != nil {
		return fmt.Errorf("write ledger entry: %w", err)
	}
	return nil
}

// exchangeCoins credits a user for cash the admin took. This is the only way
// coins are created: unlike an order, which moves existing coins between two
// users, an exchange brings new ones into the system.
func exchangeCoins(db *sql.DB, adminID, targetUserID int64, amount int, reason string) (User, error) {
	if amount <= 0 {
		return User{}, errInvalidAmount
	}

	tx, err := db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	if err := applyLedgerEntry(tx, ledgerEntry{
		UserID:  targetUserID,
		Amount:  amount,
		Kind:    KindExchangeIn,
		Reason:  reason,
		ActorID: &adminID,
	}); err != nil {
		return User{}, err
	}

	// Re-read inside the transaction so the response reflects the committed
	// balance rather than a value computed before the write.
	var u User
	if err := tx.QueryRow(`
SELECT id, name, email, role, coin_balance, password_hash
FROM users WHERE id = $1`, targetUserID,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CoinBalance, &u.PasswordHash); err != nil {
		return User{}, fmt.Errorf("reload user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return u, nil
}

// CoinEntry is one row of a user's coin history, as returned to clients.
type CoinEntry struct {
	ID        int64  `json:"id"`
	Amount    int    `json:"amount"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	ActorID   *int64 `json:"actor_id"`
	OrderID   *int64 `json:"order_id"`
	CreatedAt string `json:"created_at"`
}

func listCoinEntries(db *sql.DB, userID int64, limit int) ([]CoinEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.Query(`
SELECT id, amount, kind, reason, actor_id, order_id, created_at
FROM coin_transactions
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list coin entries: %w", err)
	}
	defer rows.Close()

	entries := []CoinEntry{}
	for rows.Next() {
		var e CoinEntry
		var createdAt any
		if err := rows.Scan(&e.ID, &e.Amount, &e.Kind, &e.Reason, &e.ActorID, &e.OrderID, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = formatTimestamp(createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
