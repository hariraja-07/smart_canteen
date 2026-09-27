package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var errUserNotFound = errors.New("user not found")
var errBadCredentials = errors.New("invalid email or password")

// createUser inserts a user, optionally crediting a starting balance. The
// balance is never written directly: it is created as an exchange_in ledger
// entry so that coin_balance always equals the sum of the user's ledger.
func createUser(db *sql.DB, name, email, password, role string, startingCoins int) (User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))

	if name == "" || email == "" {
		return User{}, errors.New("name and email are required")
	}
	if err := validRole(role); err != nil {
		return User{}, err
	}
	if len(password) < 8 {
		return User{}, errors.New("password must be at least 8 characters")
	}
	if startingCoins < 0 {
		return User{}, errors.New("starting coins cannot be negative")
	}

	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}

	tx, err := db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var u User
	u.PasswordHash = hash
	// The cached balance and the ledger entry are both derived from
	// startingCoins in the same transaction, so the two cannot disagree. Writing
	// them from separate sources is how a balance silently drifts away from its
	// ledger.
	err = tx.QueryRow(`
INSERT INTO users (name, email, password_hash, role, coin_balance)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, email, role, coin_balance`,
		name, email, hash, role, startingCoins,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CoinBalance)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, fmt.Errorf("email %q already registered", email)
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}

	if startingCoins > 0 {
		if _, err := tx.Exec(`
INSERT INTO coin_transactions (user_id, amount, kind, reason)
VALUES ($1, $2, 'exchange_in', $3)`,
			u.ID, startingCoins, "starting balance",
		); err != nil {
			return User{}, fmt.Errorf("credit starting balance: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return u, nil
}

func findUserByEmail(db *sql.DB, email string) (User, error) {
	return scanUser(db.QueryRow(`
SELECT id, name, email, role, coin_balance, password_hash
FROM users WHERE email = $1`, strings.ToLower(strings.TrimSpace(email))))
}

func findUserByID(db *sql.DB, id int64) (User, error) {
	return scanUser(db.QueryRow(`
SELECT id, name, email, role, coin_balance, password_hash
FROM users WHERE id = $1`, id))
}

// listUsers returns the roster for the admin view, highest balance first so the
// accounts that matter most are at the top.
func listUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(`
SELECT id, name, email, role, coin_balance, password_hash
FROM users
ORDER BY coin_balance DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CoinBalance, &u.PasswordHash); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func scanUser(row *sql.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CoinBalance, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, errUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// authenticate returns the user on a correct email and password. An unknown
// email and a wrong password both yield errBadCredentials, so the endpoint
// cannot be used to discover which addresses are registered.
func authenticate(db *sql.DB, email, password string) (User, error) {
	u, err := findUserByEmail(db, email)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			// Hash a throwaway value so a missing account and a wrong password
			// take a comparable amount of time.
			_ = checkPassword("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", password)
			return User{}, errBadCredentials
		}
		return User{}, err
	}
	if !checkPassword(u.PasswordHash, password) {
		return User{}, errBadCredentials
	}
	return u, nil
}

func isUniqueViolation(err error) bool {
	type sqlStater interface{ SQLState() string }
	if s, ok := err.(sqlStater); ok {
		return s.SQLState() == "23505"
	}
	return false
}
