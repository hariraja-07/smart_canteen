package main

// Everything this API reads or writes, in one place. These types are the wire
// contract: renaming a field here changes the API, and nothing else does.
//
// They are deliberately separate from the types used for database rows. The
// domain types carry a password hash and know how a row is scanned; these
// describe only what a client is allowed to see. Keeping them apart means a
// column added to a table is not silently published, and a field wanted on the
// wire is a deliberate addition rather than whatever a struct happens to hold.
//
// No field below is tagged omitempty. A client reading this contract expects
// every key to be present, and an absent key is indistinguishable from a null
// one; "actor_id": null is a fact, whereas a missing actor_id is a bug.

// ---------------------------------------------------------------- requests

// CartLine is one requested item, as sent by the client.
type CartLine struct {
	MenuItemID int64 `json:"menu_item_id"`
	Qty        int   `json:"qty"`
}

type exchangeRequest struct {
	Amount int    `json:"amount"`
	Reason string `json:"reason"`
}

type placeOrderRequest struct {
	Items []CartLine `json:"items"`
}

type setStatusRequest struct {
	Status string `json:"status"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// --------------------------------------------------------------- responses

// userResponse is a user as a client sees them. PasswordHash has no field here
// and no path to being added by accident, which is a stronger guarantee than
// the json:"-" tag it replaces.
type userResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	CoinBalance int    `json:"coin_balance"`
}

// loginResponse is flat rather than nesting the user under a "user" key, which
// is what embedding the domain type used to produce. The shape is unchanged.
type loginResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	CoinBalance int    `json:"coin_balance"`
	Token       string `json:"token"`
}

type menuItemResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Price       float64 `json:"price"`
	Description string  `json:"description"`
	Available   bool    `json:"available"`
}

type orderItemResponse struct {
	MenuItemID int64  `json:"menu_item_id"`
	Name       string `json:"name"`
	Qty        int    `json:"qty"`
	UnitPrice  int    `json:"unit_price"`
	LineTotal  int    `json:"line_total"`
}

type orderResponse struct {
	ID        int64               `json:"id"`
	UserID    int64               `json:"user_id"`
	Customer  string              `json:"customer"`
	Total     int                 `json:"total"`
	Status    string              `json:"status"`
	Items     []orderItemResponse `json:"items"`
	CreatedAt string              `json:"created_at"`
	UpdatedAt string              `json:"updated_at"`
}

type coinEntryResponse struct {
	ID        int64  `json:"id"`
	Amount    int    `json:"amount"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	ActorID   *int64 `json:"actor_id"`
	OrderID   *int64 `json:"order_id"`
	CreatedAt string `json:"created_at"`
}

// -------------------------------------------------------------- conversions

func userResponseOf(u User) userResponse {
	return userResponse{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		Role:        u.Role,
		CoinBalance: u.CoinBalance,
	}
}

func userResponsesOf(users []User) []userResponse {
	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userResponseOf(u))
	}
	return out
}

func loginResponseOf(u User, token string) loginResponse {
	return loginResponse{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		Role:        u.Role,
		CoinBalance: u.CoinBalance,
		Token:       token,
	}
}

func menuItemResponseOf(m MenuItem) menuItemResponse {
	return menuItemResponse{
		ID:          m.ID,
		Name:        m.Name,
		Category:    m.Category,
		Price:       m.Price,
		Description: m.Description,
		Available:   m.Available,
	}
}

func menuItemResponsesOf(items []MenuItem) []menuItemResponse {
	out := make([]menuItemResponse, 0, len(items))
	for _, m := range items {
		out = append(out, menuItemResponseOf(m))
	}
	return out
}

func orderResponseOf(o Order) orderResponse {
	items := make([]orderItemResponse, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, orderItemResponse{
			MenuItemID: it.MenuItemID,
			Name:       it.Name,
			Qty:        it.Qty,
			UnitPrice:  it.UnitPrice,
			LineTotal:  it.LineTotal,
		})
	}
	return orderResponse{
		ID:        o.ID,
		UserID:    o.UserID,
		Customer:  o.Customer,
		Total:     o.Total,
		Status:    o.Status,
		Items:     items,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}

func orderResponsesOf(orders []Order) []orderResponse {
	out := make([]orderResponse, 0, len(orders))
	for _, o := range orders {
		out = append(out, orderResponseOf(o))
	}
	return out
}

func coinEntryResponseOf(e CoinEntry) coinEntryResponse {
	return coinEntryResponse{
		ID:        e.ID,
		Amount:    e.Amount,
		Kind:      e.Kind,
		Reason:    e.Reason,
		ActorID:   e.ActorID,
		OrderID:   e.OrderID,
		CreatedAt: e.CreatedAt,
	}
}

func coinEntryResponsesOf(entries []CoinEntry) []coinEntryResponse {
	out := make([]coinEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, coinEntryResponseOf(e))
	}
	return out
}
