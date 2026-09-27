package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// formatTimestamp renders a timestamp column as RFC3339 for JSON responses.
func formatTimestamp(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	os.Exit(runCLI(os.Args[1:]))
}

// runServe starts the HTTP server. It is the default command, so running the
// binary with no arguments behaves as before.
func runServe() int {
	db, err := initDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	secret, err := jwtSecret()
	if err != nil {
		log.Fatal(err)
	}

	port, err := listenPort()
	if err != nil {
		log.Fatal(err)
	}
	addr := ":" + port
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newRouter(db, secret)))
	return 0
}

// listenPort returns the TCP port to bind, from PORT when the host sets it.
//
// Hosted platforms (Render, Fly, Cloud Run) assign the port through PORT and
// route traffic to whatever it says. Binding a fixed 8080 there means the
// service is up and healthy while every request goes somewhere else, so it gets
// killed and restarted in a loop. Defaulting to 8080 keeps `go run .` and
// Docker working unchanged.
//
// An unusable PORT is reported rather than ignored: silently falling back to
// 8080 would hide the misconfiguration and reintroduce the exact bug this
// function exists to prevent.
func listenPort() (string, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return "8080", nil
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", fmt.Errorf("PORT=%q is not a valid port number", port)
	}
	return port, nil
}

// newRouter builds the whole API. It is separated from runServe so tests can
// mount the real routes on an httptest server instead of exercising a socket.
func newRouter(db *sql.DB, secret string) http.Handler {
	mux := http.NewServeMux()
	// "/{$}" matches the root and nothing else. A bare "/" is a catch-all, which
	// would answer every unmatched path and every wrong method with 200 "Hello
	// World" before any auth ran, hiding typos and missed guards. With the root
	// pinned, ServeMux can return a real 404 or 405.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello World")
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /api/menu", func(w http.ResponseWriter, r *http.Request) {
		items, err := listMenuItems(db)
		if err != nil {
			log.Printf("menu: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})
	mux.HandleFunc("/api/auth/login", loginHandler(db, secret))

	// Authenticated: who am I.
	mux.Handle("GET /api/me", requireAuth(db, secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, user)
	})))

	// Admin only: the roster.
	mux.Handle("GET /api/admin/users", requireRole(db, secret, []string{RoleAdmin},
		http.HandlerFunc(listUsersHandler(db))))

	// Admin only: exchange cash for coins, and read a user's coin history.
	mux.Handle("POST /api/admin/users/{id}/coins", requireRole(db, secret, []string{RoleAdmin},
		http.HandlerFunc(exchangeCoinsHandler(db))))
	mux.Handle("GET /api/users/{id}/coins", requireAuth(db, secret,
		http.HandlerFunc(coinHistoryHandler(db))))

	// Orders. Any authenticated user can place one and read their own; the
	// canteen and admin can read the whole queue and move orders along it.
	mux.Handle("POST /api/orders", requireAuth(db, secret,
		http.HandlerFunc(placeOrderHandler(db))))
	mux.Handle("GET /api/orders", requireAuth(db, secret,
		http.HandlerFunc(listOrdersHandler(db))))
	mux.Handle("PATCH /api/orders/{id}/status", requireRole(db, secret,
		[]string{RoleCanteenManagement, RoleAdmin},
		http.HandlerFunc(setOrderStatusHandler(db))))

	return withCORS(mux)
}

type exchangeRequest struct {
	Amount int    `json:"amount"`
	Reason string `json:"reason"`
}

func exchangeCoinsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		targetID, ok := pathID(w, r, "id")
		if !ok {
			return
		}

		var req exchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.Amount <= 0 {
			writeError(w, http.StatusBadRequest, "amount must be a positive whole number of coins")
			return
		}
		if req.Reason == "" {
			// Without this the ledger records that coins appeared but not why,
			// which is the one thing it exists to explain.
			writeError(w, http.StatusBadRequest, "reason is required")
			return
		}

		target, err := exchangeCoins(db, admin.ID, targetID, req.Amount, req.Reason)
		if err != nil {
			switch {
			case errors.Is(err, errUserNotFound):
				writeError(w, http.StatusNotFound, "user not found")
			case errors.Is(err, errInvalidAmount):
				writeError(w, http.StatusBadRequest, "amount must be a positive whole number of coins")
			default:
				log.Printf("exchange coins: %v", err)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		writeJSON(w, http.StatusCreated, target)
	}
}

// coinHistoryHandler returns a user's coin history. A user may read their own;
// anyone else needs to be an admin or the canteen, which is the role that
// reconciles against these numbers.
func coinHistoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		targetID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if viewer.ID != targetID && viewer.Role != RoleAdmin && viewer.Role != RoleCanteenManagement {
			writeError(w, http.StatusForbidden, "you may only read your own coin history")
			return
		}

		entries, err := listCoinEntries(db, targetID, 100)
		if err != nil {
			log.Printf("coin history: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, entries)
	}
}

// pathID reads a {name} path value as a positive integer, writing the error
// response itself when the value is unusable.
func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

type placeOrderRequest struct {
	Items []CartLine `json:"items"`
}

func placeOrderHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		var req placeOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		order, err := placeOrder(db, user.ID, req.Items)
		if err != nil {
			switch {
			case errors.Is(err, errEmptyCart):
				writeError(w, http.StatusBadRequest, "order has no items")
			case errors.Is(err, errInsufficientCoins):
				// 402: the request was well formed but there is not enough balance.
				writeError(w, http.StatusPaymentRequired, "not enough coins")
			case errors.Is(err, errInvalidAmount):
				writeError(w, http.StatusBadRequest, "quantity must be a positive whole number")
			case errors.Is(err, errItemUnavailable):
				// Prices come from the database, so an unavailable item is sold
				// out or withdrawn, and the client should hear which.
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				// Anything else is ours: a deadlock, a dropped connection, a bad
				// query. Reporting those as 400 would tell the client its request
				// was malformed when retrying is the right response, so the detail
				// goes to the log and the client gets a generic 500.
				log.Printf("place order: %v", err)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		writeJSON(w, http.StatusCreated, order)
	}
}

func listOrdersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		orders, err := listOrders(db, viewer, r.URL.Query().Get("status"), 50)
		if err != nil {
			if strings.Contains(err.Error(), "unknown status") {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			log.Printf("list orders: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, orders)
	}
}

type setStatusRequest struct {
	Status string `json:"status"`
}

func setOrderStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var req setStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		order, err := setOrderStatus(db, orderID, req.Status)
		if err != nil {
			switch {
			case errors.Is(err, errOrderNotFound):
				writeError(w, http.StatusNotFound, "order not found")
			case errors.Is(err, errInsufficientCoins):
				writeError(w, http.StatusConflict, "canteen balance cannot cover this refund")
			case strings.Contains(err.Error(), "unknown status"),
				strings.Contains(err.Error(), "cannot move an order"):
				writeError(w, http.StatusConflict, err.Error())
			default:
				log.Printf("set order status: %v", err)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		writeJSON(w, http.StatusOK, order)
	}
}

func listUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		users, err := listUsers(db)
		if err != nil {
			log.Printf("list users: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	User
	Token string `json:"token"`
}

func loginHandler(db *sql.DB, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		user, err := authenticate(db, req.Email, req.Password)
		if err != nil {
			if errors.Is(err, errBadCredentials) {
				// Same response either way: never reveal which emails exist.
				writeError(w, http.StatusUnauthorized, "invalid email or password")
				return
			}
			log.Printf("login: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		token, err := signToken(user, secret)
		if err != nil {
			log.Printf("login: sign token: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, loginResponse{User: user, Token: token})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
