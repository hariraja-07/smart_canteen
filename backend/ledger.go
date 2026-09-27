package main

import (
	"database/sql"
	"fmt"
	"os"
)

// BalanceDrift is a user whose cached coin_balance disagrees with the sum of
// their ledger entries. It should never be non-zero: every balance change in
// the system goes through a transaction that writes both, so a non-zero result
// means a code path updated one and not the other.
type BalanceDrift struct {
	Email     string
	Cached    int
	LedgerSum int
}

// checkLedgerInvariants returns every user whose cached balance has drifted
// from their ledger, and the total number of coins that a transfer should have
// left unchanged but did not.
func checkLedgerInvariants(db *sql.DB) ([]BalanceDrift, error) {
	rows, err := db.Query(`
SELECT u.email,
       u.coin_balance,
       COALESCE(SUM(ct.amount), 0)
FROM users u
LEFT JOIN coin_transactions ct ON ct.user_id = u.id
GROUP BY u.id, u.email, u.coin_balance
HAVING u.coin_balance <> COALESCE(SUM(ct.amount), 0)
ORDER BY u.email`)
	if err != nil {
		return nil, fmt.Errorf("check balances: %w", err)
	}
	defer rows.Close()

	var drift []BalanceDrift
	for rows.Next() {
		var d BalanceDrift
		if err := rows.Scan(&d.Email, &d.Cached, &d.LedgerSum); err != nil {
			return nil, err
		}
		drift = append(drift, d)
	}
	return drift, rows.Err()
}

// unbalancedTransfers returns transfer groups that do not net to zero. An
// exchange is a single entry and legitimately stands alone; only groups with
// two or more entries are required to balance.
func unbalancedTransfers(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
SELECT group_id::text, SUM(amount)
FROM coin_transactions
GROUP BY group_id
HAVING COUNT(*) > 1 AND SUM(amount) <> 0
ORDER BY MIN(created_at)`)
	if err != nil {
		return nil, fmt.Errorf("check transfers: %w", err)
	}
	defer rows.Close()

	var bad []string
	for rows.Next() {
		var id string
		var sum int
		if err := rows.Scan(&id, &sum); err != nil {
			return nil, err
		}
		bad = append(bad, id)
	}
	return bad, rows.Err()
}

// coinSummary reports the totals a canteen manager would want to see: coins
// issued, coins spent, and what the canteen has collected.
type coinSummary struct {
	Issued         int
	Spent          int
	CanteenRevenue int
	CoinsInFlight  int
}

func summariseCoins(db *sql.DB) (coinSummary, error) {
	var s coinSummary
	err := db.QueryRow(`
SELECT
	COALESCE(SUM(amount) FILTER (WHERE kind = 'exchange_in'), 0),
	COALESCE(-SUM(amount) FILTER (WHERE kind = 'order_payment'), 0),
	COALESCE(SUM(amount) FILTER (WHERE kind = 'canteen_revenue'), 0),
	COALESCE(SUM(amount), 0)
FROM coin_transactions`).Scan(&s.Issued, &s.Spent, &s.CanteenRevenue, &s.CoinsInFlight)
	if err != nil {
		return coinSummary{}, fmt.Errorf("summarise coins: %w", err)
	}
	return s, nil
}

func runReconcile() int {
	db, err := initDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer db.Close()

	drift, err := checkLedgerInvariants(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	unbalanced, err := unbalancedTransfers(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	for _, d := range drift {
		fmt.Printf("DRIFT   %s: cached %d, ledger %d (off by %d)\n",
			d.Email, d.Cached, d.LedgerSum, d.LedgerSum-d.Cached)
	}
	for _, id := range unbalanced {
		fmt.Printf("DRIFT   transfer %s does not net to zero\n", id)
	}

	summary, err := summariseCoins(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("issued %d coins, spent %d, canteen revenue %d, in circulation %d\n",
		summary.Issued, summary.Spent, summary.CanteenRevenue, summary.CoinsInFlight)

	if len(drift) == 0 && len(unbalanced) == 0 {
		fmt.Println("invariants OK: every balance matches its ledger")
		return 0
	}
	return 1
}
