package main

// The shapes this API accepts. These are decoded from request bodies; nothing
// here is also used as a database row.

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

type loginResponse struct {
	User
	Token string `json:"token"`
}
