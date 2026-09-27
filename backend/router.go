package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
)

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
		writeJSON(w, http.StatusOK, menuItemResponsesOf(items))
	})
	mux.HandleFunc("/api/auth/login", loginHandler(db, secret))

	// Authenticated: who am I.
	mux.Handle("GET /api/me", requireAuth(db, secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, userResponseOf(user))
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
