package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello World")
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/api/menu", func(w http.ResponseWriter, r *http.Request) {
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
	mux.Handle("/api/me", requireAuth(db, secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, user)
	})))

	// Admin only: the roster.
	mux.Handle("/api/admin/users", requireRole(db, secret, []string{RoleAdmin},
		http.HandlerFunc(listUsersHandler(db))))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", withCORS(mux)))
	return 0
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
