package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// These pin the JSON contract itself: which keys each response carries, and
// which it must never carry. They need no database and must keep running when
// there is none.

// keysOf marshals v and returns its top-level keys, so a test can assert the
// exact set rather than spot-checking the fields it happens to care about.
func keysOf(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal into object: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func assertKeys(t *testing.T, v any, want []string) {
	t.Helper()
	g, w := slices.Clone(keysOf(t, v)), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if !reflect.DeepEqual(g, w) {
		t.Errorf("keys: got %v, want %v", g, w)
	}
}

// A hash on the row must not be reachable from the response. The old User type
// relied on a json:"-" tag for this, which is one forgotten tag away from
// leaking every user's password hash to anyone who can read /api/me. The
// response type has no such field, so there is nothing to forget.
func TestUserResponseNeverCarriesThePasswordHash(t *testing.T) {
	u := User{ID: 1, Name: "A", Email: "a@b.c", Role: RoleStudent, CoinBalance: 10,
		PasswordHash: "$2a$10$supersecret"}

	if body, err := json.Marshal(userResponseOf(u)); err != nil {
		t.Fatalf("marshal: %v", err)
	} else if strings.Contains(string(body), "supersecret") || strings.Contains(string(body), "password") {
		t.Errorf("response leaked the password hash: %s", body)
	}

	assertKeys(t, userResponseOf(u), []string{"id", "name", "email", "role", "coin_balance"})
}

// Login flattens the user alongside the token, because it always has and the
// client parses it that way. Nesting it under a "user" key would parse as a
// User with every field null, and the failure would look like missing data
// rather than a shape change.
func TestLoginResponseIsFlat(t *testing.T) {
	u := User{ID: 7, Name: "A", Email: "a@b.c", Role: RoleStaff, CoinBalance: 3,
		PasswordHash: "hash"}
	got := loginResponseOf(u, "tok")

	assertKeys(t, got, []string{"id", "name", "email", "role", "coin_balance", "token"})

	raw, _ := json.Marshal(got)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["token"] != "tok" {
		t.Errorf("token: got %v, want %q", m["token"], "tok")
	}
	if m["coin_balance"] != float64(3) {
		t.Errorf("coin_balance: got %v, want 3", m["coin_balance"])
	}
}

// Every key is always present, so a client can tell "no actor" from "this
// response is from an older server that never had the field". Omitting a nil
// pointer instead of writing null would make those two indistinguishable.
func TestNilPointersAreNullRatherThanAbsent(t *testing.T) {
	got := coinEntryResponseOf(CoinEntry{ID: 1, Amount: -10, Kind: "order_payment",
		Reason: "order 3"})

	assertKeys(t, got, []string{"id", "amount", "kind", "reason", "actor_id", "order_id", "created_at"})

	raw, _ := json.Marshal(got)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"actor_id", "order_id"} {
		if string(m[key]) != "null" {
			t.Errorf("%s: got %s, want null", key, m[key])
		}
	}
}

// A present pointer must still round-trip as a number, not as a string or a
// nested object.
func TestSetPointersSurviveAsNumbers(t *testing.T) {
	actor, order := int64(4), int64(9)
	got := coinEntryResponseOf(CoinEntry{ID: 2, Amount: 10, Kind: "exchange_in",
		ActorID: &actor, OrderID: &order})

	raw, _ := json.Marshal(got)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["actor_id"] != float64(4) || m["order_id"] != float64(9) {
		t.Errorf("got actor_id=%v order_id=%v, want 4 and 9", m["actor_id"], m["order_id"])
	}
}

// Order lines are nested under "items", and an order with no lines must send an
// empty array rather than null, since the client maps over the field.
func TestOrderResponseNestsItsLines(t *testing.T) {
	empty := orderResponseOf(Order{ID: 1, UserID: 2, Status: "pending"})
	if empty.Items == nil {
		t.Error("items: got null, want an empty array so the client can map over it")
	}

	one := orderResponseOf(Order{ID: 1, UserID: 2, Status: "pending", Items: []OrderItem{
		{MenuItemID: 5, Name: "Dosa", Qty: 2, UnitPrice: 30, LineTotal: 60},
	}})
	assertKeys(t, one, []string{"id", "user_id", "customer", "total", "status", "items",
		"created_at", "updated_at"})

	raw, _ := json.Marshal(one.Items[0])
	assertKeys(t, mustUnmarshal(t, raw), []string{"menu_item_id", "name", "qty", "unit_price", "line_total"})
}

func TestMenuItemResponseKeys(t *testing.T) {
	assertKeys(t, menuItemResponseOf(MenuItem{ID: 1, Name: "Dosa", Category: "main",
		Price: 30, Available: true}),
		[]string{"id", "name", "category", "price", "description", "available"})
}

// mustUnmarshal decodes raw into a generic object, failing the test if it cannot.
func mustUnmarshal(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return m
}
