package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

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
		writeJSON(w, http.StatusCreated, userResponseOf(target))
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
		writeJSON(w, http.StatusOK, coinEntryResponsesOf(entries))
	}
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
		writeJSON(w, http.StatusCreated, orderResponseOf(order))
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
		writeJSON(w, http.StatusOK, orderResponsesOf(orders))
	}
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
		writeJSON(w, http.StatusOK, orderResponseOf(order))
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
		writeJSON(w, http.StatusOK, userResponsesOf(users))
	}
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
		writeJSON(w, http.StatusOK, loginResponseOf(user, token))
	}
}
