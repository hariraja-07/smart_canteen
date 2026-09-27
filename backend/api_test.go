package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// These are integration tests: they need a real Postgres, because the
// behaviour under test is largely database behaviour, including the row locks
// that stop two orders spending the same coins. Point TEST_DATABASE_URL at a
// throwaway database; without it the suite skips rather than failing.

const testSecret = "test-secret-that-is-long-enough-32"

var testDB *sql.DB

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set, skipping integration tests")
		os.Exit(0)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	if err := runMigrations(db); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

// --- helpers ---

var testMu sync.Mutex
var testSeq int

// testRunID keeps emails unique across runs. A plain counter would restart at 1
// while the previous run's users are still in the database, so a second run
// would collide on the unique email constraint instead of testing anything.
var testRunID = func() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}()

// newTestUser creates a uniquely named account so tests never collide on the
// unique email constraint.
func newTestUser(t *testing.T, role string, coins int) User {
	t.Helper()
	testMu.Lock()
	testSeq++
	n := testSeq
	testMu.Unlock()

	name := fmt.Sprintf("test-%s-%d", role, n)
	email := fmt.Sprintf("test-%s-%d-%s@example.test", testRunID, n, role)
	u, err := createUser(testDB, name, email, "testpassword123", role, coins)
	if err != nil {
		t.Fatalf("createUser(%s): %v", role, err)
	}
	return u
}

func tokenFor(t *testing.T, u User) string {
	t.Helper()
	tok, err := signToken(u, testSecret)
	if err != nil {
		t.Fatalf("signToken: %v", err)
	}
	return tok
}

// assertLedgerConsistent is the central invariant of the whole system: every
// cached balance equals the sum of that user's ledger, and no group of related
// entries fails to net to zero. Every test that moves money ends with this.
func assertLedgerConsistent(t *testing.T) {
	t.Helper()
	rows, err := testDB.Query(`
SELECT u.email, u.coin_balance, COALESCE(SUM(t.amount), 0)
FROM users u LEFT JOIN coin_transactions t ON t.user_id = u.id
GROUP BY u.id, u.email, u.coin_balance`)
	if err != nil {
		t.Fatalf("reconcile query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var email string
		var cached, ledger int
		if err := rows.Scan(&email, &cached, &ledger); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if cached != ledger {
			t.Errorf("balance drift for %s: cached %d, ledger %d", email, cached, ledger)
		}
		if cached < 0 {
			t.Errorf("%s has a negative balance %d", email, cached)
		}
	}

	bad, err := unbalancedTransfers(testDB)
	if err != nil {
		t.Fatalf("unbalancedTransfers: %v", err)
	}
	if len(bad) != 0 {
		t.Errorf("unbalanced transfers: %+v", bad)
	}
}

func doJSON(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, out.Bytes()
}

func decodeInto(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode %q: %v", string(raw), err)
	}
}

// a cheap menu item price for arithmetic in tests
func someMenuItem(t *testing.T) (int64, int) {
	t.Helper()
	var id int64
	var price int
	err := testDB.QueryRow(
		`SELECT id, price::INT FROM menu_items WHERE available ORDER BY price LIMIT 1`).Scan(&id, &price)
	if err != nil {
		t.Skipf("no menu items available: %v", err)
	}
	return id, price
}

func canteenUser(t *testing.T) User {
	t.Helper()
	return newTestUser(t, RoleCanteenManagement, 0)
}

// --- auth ---

func TestLoginRejectsBadCredentials(t *testing.T) {
	u := newTestUser(t, RoleStudent, 10)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	cases := []struct {
		name, email, password string
	}{
		{"wrong password", u.Email, "not-the-password"},
		{"unknown user", "nobody@example.test", "whatever123"},
		{"empty password", u.Email, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, body := doJSON(t, "POST", srv.URL+"/api/auth/login", "",
				map[string]string{"email": c.email, "password": c.password})
			// An unknown user and a wrong password must be indistinguishable, or
			// the response becomes a way to enumerate accounts.
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401 (body %s)", resp.StatusCode, body)
			}
			var out map[string]any
			decodeInto(t, body, &out)
			if msg, _ := out["error"].(string); msg != "invalid email or password" {
				t.Errorf("error message %q reveals too much or is wrong", msg)
			}
		})
	}
}

func TestProtectedRoutesRejectMissingToken(t *testing.T) {
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()
	for _, path := range []string{
		"/api/me", "/api/orders", "/api/admin/users", "/api/users/1/coins",
	} {
		t.Run(path, func(t *testing.T) {
			resp, _ := doJSON(t, "GET", srv.URL+path, "", nil)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("got %d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestRoleGuards(t *testing.T) {
	student := newTestUser(t, RoleStudent, 0)
	canteen := newTestUser(t, RoleCanteenManagement, 0)
	admin := newTestUser(t, RoleAdmin, 0)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	st := tokenFor(t, student)
	ct := tokenFor(t, canteen)
	ad := tokenFor(t, admin)

	// admin-only route
	for _, c := range []struct {
		role string
		tok  string
		want int
	}{
		{"student", st, http.StatusForbidden},
		{"canteen", ct, http.StatusForbidden},
		{"admin", ad, http.StatusOK},
	} {
		resp, _ := doJSON(t, "GET", srv.URL+"/api/admin/users", c.tok, nil)
		if resp.StatusCode != c.want {
			t.Errorf("%s reading roster: got %d, want %d", c.role, resp.StatusCode, c.want)
		}
	}

	// Advancing an order is canteen/admin only. This needs a real order, since a
	// request for a missing one is refused with 404 before the role is considered,
	// and a fresh order per role, since advancing an already-advanced order is a
	// 409 from the state machine rather than a role decision.
	item, _ := someMenuItem(t)
	for _, c := range []struct {
		role string
		tok  string
		want int
	}{
		{"student", st, http.StatusForbidden},
		{"canteen", ct, http.StatusOK},
		{"admin", ad, http.StatusOK},
	} {
		owner := newTestUser(t, RoleStudent, 500)
		_, body := doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, owner),
			map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
		var order Order
		decodeInto(t, body, &order)
		if order.ID == 0 {
			t.Fatalf("could not place a test order: %s", body)
		}
		resp, _ := doJSON(t, "PATCH",
			fmt.Sprintf("%s/api/orders/%d/status", srv.URL, order.ID), c.tok,
			map[string]string{"status": StatusPreparing})
		if resp.StatusCode != c.want {
			t.Errorf("%s advancing order: got %d, want %d", c.role, resp.StatusCode, c.want)
		}
	}
}

func TestCoinHistoryIsScoped(t *testing.T) {
	owner := newTestUser(t, RoleStudent, 50)
	other := newTestUser(t, RoleStudent, 0)
	admin := newTestUser(t, RoleAdmin, 0)
	canteen := newTestUser(t, RoleCanteenManagement, 0)

	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	// owner, admin and canteen may read; another student may not
	for _, c := range []struct {
		role string
		who  User
		want int
	}{
		{"owner", owner, http.StatusOK},
		{"admin", admin, http.StatusOK},
		{"canteen", canteen, http.StatusOK},
		{"other student", other, http.StatusForbidden},
	} {
		tok := tokenFor(t, c.who)
		resp, _ := doJSON(t, "GET",
			fmt.Sprintf("%s/api/users/%d/coins", srv.URL, owner.ID), tok, nil)
		if resp.StatusCode != c.want {
			t.Errorf("%s reading %s's history: got %d, want %d",
				c.role, owner.Email, resp.StatusCode, c.want)
		}
	}
}

func TestResponseNeverLeaksPasswordHash(t *testing.T) {
	u := newTestUser(t, RoleStudent, 5)
	admin := newTestUser(t, RoleAdmin, 0)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	_, body := doJSON(t, "GET", srv.URL+"/api/me", tokenFor(t, u), nil)
	if strings.Contains(string(body), "password") {
		t.Errorf("/api/me leaked a password field: %s", body)
	}
	_, body = doJSON(t, "GET", srv.URL+"/api/admin/users", tokenFor(t, admin), nil)
	if strings.Contains(string(body), "password") {
		t.Errorf("/api/admin/users leaked a password field: %s", body)
	}
}

// --- routing ---

func TestUnmatchedPathIsNotSilentlySuccessful(t *testing.T) {
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()
	// A bare "/" catch-all once answered these with 200 "Hello World" before any
	// auth ran, which would have hidden a guard added to the wrong path.
	// DELETE is registered for none of these paths, so ServeMux must reject it
	// with 405 rather than falling through to a catch-all.
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/nope", http.StatusNotFound},
		{"DELETE", "/api/orders", http.StatusMethodNotAllowed},
		{"DELETE", "/api/me", http.StatusMethodNotAllowed},
		{"GET", "/api/admin/users/1/coins", http.StatusMethodNotAllowed},
	} {
		resp, _ := doJSON(t, c.method, srv.URL+c.path, "", map[string]string{"status": "preparing"})
		if resp.StatusCode != c.want {
			t.Errorf("%s %s: got %d, want %d", c.method, c.path, resp.StatusCode, c.want)
		}
	}
}

// --- coin exchange ---

func TestExchangeValidation(t *testing.T) {
	admin := newTestUser(t, RoleAdmin, 0)
	target := newTestUser(t, RoleStudent, 0)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()
	tok := tokenFor(t, admin)
	url := fmt.Sprintf("%s/api/admin/users/%d/coins", srv.URL, target.ID)

	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"zero", map[string]any{"amount": 0, "reason": "x"}, http.StatusBadRequest},
		{"negative", map[string]any{"amount": -5, "reason": "x"}, http.StatusBadRequest},
		{"fractional", map[string]any{"amount": 1.5, "reason": "x"}, http.StatusBadRequest},
		{"no reason", map[string]any{"amount": 5}, http.StatusBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			resp, _ := doJSON(t, "POST", url, tok, c.body)
			if resp.StatusCode != c.want {
				t.Errorf("got %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
	assertLedgerConsistent(t)
}

func TestExchangeMovesBothCacheAndLedger(t *testing.T) {
	admin := newTestUser(t, RoleAdmin, 0)
	target := newTestUser(t, RoleStudent, 0)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	resp, body := doJSON(t, "POST",
		fmt.Sprintf("%s/api/admin/users/%d/coins", srv.URL, target.ID),
		tokenFor(t, admin),
		map[string]any{"amount": 25, "reason": "paid Rs 25 cash"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body %s)", resp.StatusCode, body)
	}
	var u User
	decodeInto(t, body, &u)
	if u.CoinBalance != 25 {
		t.Errorf("balance %d, want 25", u.CoinBalance)
	}
	assertLedgerConsistent(t)
}

// --- orders ---

func TestPlaceOrderDebitsStudentAndCreditsCanteen(t *testing.T) {
	canteen := canteenUser(t)
	student := newTestUser(t, RoleStudent, 0)
	item, price := someMenuItem(t)

	canteenBefore := canteen.CoinBalance
	admin := newTestUser(t, RoleAdmin, 0)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()
	adminTok := tokenFor(t, admin)

	// fund the student through the real endpoint, not by writing the table
	doJSON(t, "POST", fmt.Sprintf("%s/api/admin/users/%d/coins", srv.URL, student.ID),
		adminTok, map[string]any{"amount": price * 3, "reason": "test funding"})

	resp, body := doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, student),
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 2}}})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body %s)", resp.StatusCode, body)
	}
	var order Order
	decodeInto(t, body, &order)
	if order.Total != price*2 {
		t.Errorf("total %d, want %d", order.Total, price*2)
	}

	// the two halves of the transfer must share a group and net to zero
	rows, err := testDB.Query(`
SELECT kind, amount, group_id FROM coin_transactions WHERE order_id = $1 ORDER BY kind`, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sum int
	var groups int
	for rows.Next() {
		var kind, group string
		var amount int
		rows.Scan(&kind, &amount, &group)
		sum += amount
		groups++
		if group == "" {
			t.Error("ledger entry has an empty group id, so the transfer is not linkable")
		}
	}
	if sum != 0 {
		t.Errorf("order %d ledger sums to %d, want 0: an order must move coins, not create them", order.ID, sum)
	}
	if groups != 2 {
		t.Errorf("got %d ledger entries for one order, want 2", groups)
	}
	assertLedgerConsistent(t)
	_ = canteenBefore
}

func TestPlaceOrderRejectsBadCarts(t *testing.T) {
	student := newTestUser(t, RoleStudent, 0)
	item, _ := someMenuItem(t)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()
	tok := tokenFor(t, student)

	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"empty cart", map[string]any{"items": []any{}}, http.StatusBadRequest},
		{"zero qty", map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 0}}}, http.StatusBadRequest},
		{"negative qty", map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": -1}}}, http.StatusBadRequest},
		{"unknown item", map[string]any{"items": []map[string]any{{"menu_item_id": 999999, "qty": 1}}}, http.StatusBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			resp, _ := doJSON(t, "POST", srv.URL+"/api/orders", tok, c.body)
			if resp.StatusCode != c.want {
				t.Errorf("got %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
	assertLedgerConsistent(t)
}

func TestOrderBeyondBalanceIsRejected(t *testing.T) {
	student := newTestUser(t, RoleStudent, 0)
	item, price := someMenuItem(t)
	admin := newTestUser(t, RoleAdmin, 0)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	// give the student less than one order costs
	doJSON(t, "POST", fmt.Sprintf("%s/api/admin/users/%d/coins", srv.URL, student.ID),
		tokenFor(t, admin), map[string]any{"amount": price - 1, "reason": "almost enough"})

	resp, _ := doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, student),
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("got %d, want 402", resp.StatusCode)
	}
	assertLedgerConsistent(t)
}

func TestListOrdersIsScopedByRole(t *testing.T) {
	ravi := newTestUser(t, RoleStudent, 500)
	priya := newTestUser(t, RoleStudent, 500)
	canteen := canteenUser(t)
	admin := newTestUser(t, RoleAdmin, 0)
	item, _ := someMenuItem(t)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, ravi),
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
	doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, priya),
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})

	count := func(tok string) int {
		resp, body := doJSON(t, "GET", srv.URL+"/api/orders", tok, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list orders: %d %s", resp.StatusCode, body)
		}
		var orders []Order
		decodeInto(t, body, &orders)
		return len(orders)
	}
	// Each test student must only ever see their own order, never the other's.
	if n := count(tokenFor(t, ravi)); n != 1 {
		t.Errorf("student sees %d orders, want 1", n)
	}
	if n := count(tokenFor(t, priya)); n != 1 {
		t.Errorf("student sees %d orders, want 1", n)
	}
	// The canteen and admin see at least both.
	for _, c := range []struct {
		role string
		who  User
	}{{"canteen", canteen}, {"admin", admin}} {
		if n := count(tokenFor(t, c.who)); n < 2 {
			t.Errorf("%s sees %d orders, want at least 2", c.role, n)
		}
	}
}

func TestStatusTransitionsFollowStateMachine(t *testing.T) {
	canteen := canteenUser(t)
	student := newTestUser(t, RoleStudent, 500)
	item, _ := someMenuItem(t)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	_, body := doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, student),
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
	var order Order
	decodeInto(t, body, &order)
	tok := tokenFor(t, canteen)
	advance := func(status string) int {
		resp, _ := doJSON(t, "PATCH",
			fmt.Sprintf("%s/api/orders/%d/status", srv.URL, order.ID), tok,
			map[string]string{"status": status})
		return resp.StatusCode
	}

	if got := advance(StatusCompleted); got != http.StatusConflict {
		t.Errorf("pending->completed: got %d, want 409", got)
	}
	for _, step := range []string{StatusPreparing, StatusReady, StatusCompleted} {
		if got := advance(step); got != http.StatusOK {
			t.Fatalf("advancing to %s: got %d, want 200", step, got)
		}
	}
	// completed is terminal
	if got := advance(StatusReady); got != http.StatusConflict {
		t.Errorf("completed->ready: got %d, want 409", got)
	}
	if got := advance(StatusCancelled); got != http.StatusConflict {
		t.Errorf("completed->cancelled: got %d, want 409", got)
	}
}

func TestCancelRefundsExactlyOnce(t *testing.T) {
	canteen := canteenUser(t)
	student := newTestUser(t, RoleStudent, 500)
	admin := newTestUser(t, RoleAdmin, 0)
	item, price := someMenuItem(t)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	_, body := doJSON(t, "POST", srv.URL+"/api/orders", tokenFor(t, student),
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
	var order Order
	decodeInto(t, body, &order)

	balance := func(u User) int {
		resp, b := doJSON(t, "GET", srv.URL+"/api/admin/users", tokenFor(t, admin), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("reading roster: %d", resp.StatusCode)
		}
		var users []User
		decodeInto(t, b, &users)
		for _, x := range users {
			if x.ID == u.ID {
				return x.CoinBalance
			}
		}
		return -1
	}
	before := balance(student)
	canteenBefore := balance(canteen)

	tok := tokenFor(t, canteen)
	resp, _ := doJSON(t, "PATCH",
		fmt.Sprintf("%s/api/orders/%d/status", srv.URL, order.ID), tok,
		map[string]string{"status": StatusCancelled})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel: got %d", resp.StatusCode)
	}
	if got := balance(student); got != before+price {
		t.Errorf("after cancel student has %d, want %d", got, before+price)
	}
	// The canteen is handed the coins when the order is placed and gives them
	// back on cancellation, so it returns to where it started. It must not go
	// negative: the refund takes back exactly what the order paid in.
	if got := balance(canteen); got != canteenBefore {
		t.Errorf("after cancel canteen has %d, want %d (its balance before the order)", got, canteenBefore)
	}

	// A second cancel must be refused and must not pay out again.
	resp, _ = doJSON(t, "PATCH",
		fmt.Sprintf("%s/api/orders/%d/status", srv.URL, order.ID), tok,
		map[string]string{"status": StatusCancelled})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second cancel: got %d, want 409", resp.StatusCode)
	}
	if got := balance(student); got != before+price {
		t.Errorf("double cancel changed the balance to %d, want %d", got, before+price)
	}
	assertLedgerConsistent(t)
}

func TestUnavailableItemIsAClientErrorNotAServerError(t *testing.T) {
	// A sold-out or withdrawn item is the caller's problem, so 400. A deadlock or
	// a dropped connection is ours and must be 500, because retrying is the right
	// client response. Collapsing both into 400 would tell a user their cart was
	// malformed when the request was fine.
	student := newTestUser(t, RoleStudent, 500)
	item, _ := someMenuItem(t)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()
	tok := tokenFor(t, student)

	// withdraw an item, then order it
	restore, err := testDB.Exec(`UPDATE menu_items SET available = false WHERE id = $1`, item)
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Exec(`UPDATE menu_items SET available = true WHERE id = $1`, item)
	_ = restore

	resp, body := doJSON(t, "POST", srv.URL+"/api/orders", tok,
		map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("sold-out item: got %d, want 400 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "sold out") {
		t.Errorf("response should say the item is sold out, got %s", body)
	}
}

func TestConcurrentOrdersCannotOverspend(t *testing.T) {
	// This is the regression test for the FK KEY SHARE deadlock. orders.user_id
	// takes a KEY SHARE lock on the user; the balance update needs to upgrade it.
	// Without locking the customer first, enough concurrent orders deadlock.
	canteenUser(t)
	student := newTestUser(t, RoleStudent, 0)
	admin := newTestUser(t, RoleAdmin, 0)
	item, price := someMenuItem(t)
	srv := httptest.NewServer(newRouter(testDB, testSecret))
	defer srv.Close()

	// fund for exactly three orders
	funds := price * 3
	doJSON(t, "POST", fmt.Sprintf("%s/api/admin/users/%d/coins", srv.URL, student.ID),
		tokenFor(t, admin), map[string]any{"amount": funds, "reason": "concurrency test"})

	const attempts = 12
	tok := tokenFor(t, student)
	var wg sync.WaitGroup
	codes := make([]int, attempts)
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release all goroutines together
			resp, _ := doJSON(t, "POST", srv.URL+"/api/orders", tok,
				map[string]any{"items": []map[string]any{{"menu_item_id": item, "qty": 1}}})
			codes[i] = resp.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()

	created := 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusPaymentRequired:
			// expected for the ones that lost the race
		default:
			t.Errorf("unexpected status %d; a 500 suggests a deadlock or a lost update", c)
		}
	}
	if created != 3 {
		t.Errorf("%d orders created from %d coins at %d each, want exactly 3", created, funds, price)
	}
	assertLedgerConsistent(t)
}

func TestReconcileDetectsInjectedDrift(t *testing.T) {
	u := newTestUser(t, RoleStudent, 42)
	// corrupt the cache without a matching ledger entry
	if _, err := testDB.Exec(
		`UPDATE users SET coin_balance = coin_balance + 1 WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	drift, err := checkLedgerInvariants(testDB)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range drift {
		if d.Email == u.Email {
			found = true
		}
	}
	if !found {
		t.Errorf("reconcile did not report the injected drift for %s (drift: %+v)", u.Email, drift)
	}
	// repair so the rest of the suite is clean
	if _, err := testDB.Exec(
		`UPDATE users SET coin_balance = 42 WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	assertLedgerConsistent(t)
}
