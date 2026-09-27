package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

// demoAccount is one seeded login. Passwords are shared on purpose: these exist
// so the app can be demonstrated, not to protect anything.
type demoAccount struct {
	Email  string
	Name   string
	Role   string
	Coins  int
	Reason string
}

var demoAccounts = []demoAccount{
	{"admin@smartcanteen.local", "Admin", RoleAdmin, 0, "demo admin"},
	{"canteen@smartcanteen.local", "Canteen", RoleCanteenManagement, 0, "demo canteen account, collects order revenue"},
	{"ravi@smartcanteen.local", "Ravi", RoleStudent, 100, "demo student"},
	{"priya@smartcanteen.local", "Priya", RoleStudent, 60, "demo student"},
	{"staff@smartcanteen.local", "Suresh", RoleStaff, 50, "demo staff"},
}

// seedDemoUsers creates the demo accounts when DEMO_USER_PASSWORD is set.
//
// It is deliberately gated. Unconditionally seeding would mean every deployment
// of this code, including the public one, starts life with accounts whose
// passwords are in the source. A deployment that does not set the variable gets
// no demo accounts and must use the createuser CLI to provision anything.
func seedDemoUsers(db *sql.DB) error {
	password := os.Getenv("DEMO_USER_PASSWORD")
	if password == "" {
		return nil
	}
	if len(password) < 8 {
		return fmt.Errorf("DEMO_USER_PASSWORD must be at least 8 characters")
	}

	for _, a := range demoAccounts {
		existing, err := findUserByEmail(db, a.Email)
		if err == nil {
			// Already seeded. Leave the balance alone: it may have been spent.
			_ = existing
			continue
		}
		if err != errUserNotFound {
			return fmt.Errorf("seed demo %s: %w", a.Email, err)
		}
		if _, err := createUser(db, a.Name, a.Email, password, a.Role, a.Coins); err != nil {
			return fmt.Errorf("seed demo %s: %w", a.Email, err)
		}
		log.Printf("seeded demo account %s role=%s coins=%d", a.Email, a.Role, a.Coins)
	}
	return nil
}
