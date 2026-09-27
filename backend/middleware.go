package main

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

type contextKey string

const userContextKey contextKey = "user"

// requireAuth rejects anything without a valid Bearer token. On success the
// resolved User is on the request context for downstream handlers.
//
// The user is re-read from the database on every request rather than trusted
// from the token body. The token only carries an id, so a role change or a
// deleted account takes effect immediately instead of persisting until the
// token expires.
func requireAuth(db *sql.DB, secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		userID, err := parseToken(secret, raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		user, err := findUserByID(db, userID)
		if err != nil {
			if err == errUserNotFound {
				writeError(w, http.StatusUnauthorized, "account no longer exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireRole wraps requireAuth and additionally demands one of the given
// roles. Roles come from the database, never from the token or the request.
func requireRole(db *sql.DB, secret string, roles []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return requireAuth(db, secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !allowed[user.Role] {
			// 403, not 401: the caller is authenticated, just not permitted.
			writeError(w, http.StatusForbidden, "insufficient role")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

func userFromContext(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(userContextKey).(User)
	return user, ok
}
