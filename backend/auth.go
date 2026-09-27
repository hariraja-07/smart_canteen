package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Roles recognised by the canteen. The users_role_check constraint added in
// migration 3 enforces the same set at the database level; this map is what the
// application validates against before a role ever reaches SQL.
const (
	RoleAdmin             = "admin"
	RoleStudent           = "student"
	RoleStaff             = "staff"
	RoleCanteenManagement = "canteen_management"
)

var validRoles = map[string]bool{
	RoleAdmin:             true,
	RoleStudent:           true,
	RoleStaff:             true,
	RoleCanteenManagement: true,
}

// errInvalidRole is returned when a caller asks for a role outside validRoles.
// Registration is closed, so role is never taken from an unauthenticated
// request body: the only two places that set it are this check and the admin
// endpoint, which sits behind requireRole.
var errInvalidRole = errors.New("invalid role")

func validRole(role string) error {
	if !validRoles[role] {
		return fmt.Errorf("%w: %q", errInvalidRole, role)
	}
	return nil
}

// tokenTTL is deliberately short. A signed token cannot be revoked before it
// expires, so the window is the blast radius if one leaks.
const tokenTTL = time.Hour

type User struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	CoinBalance  int    `json:"coin_balance"`
	PasswordHash string `json:"-"`
}

// canSeeAllOrders is true for the roles that oversee the canteen rather than
// place orders in it.
func (u User) canSeeAllOrders() bool {
	return u.Role == RoleCanteenManagement || u.Role == RoleAdmin
}

func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// jwtSecret returns the signing secret or an error. There is deliberately no
// default: a hardcoded fallback would mean a deployment silently signs tokens
// with a value that is public in the source.
func jwtSecret() (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", errors.New("JWT_SECRET is required (see backend/.env.example)")
	}
	if len(secret) < 32 {
		return "", errors.New("JWT_SECRET must be at least 32 characters")
	}
	return secret, nil
}

func signToken(u User, secret string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", u.ID),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		Issuer:    "smart-canteen",
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

var errInvalidToken = errors.New("invalid token")

// parseToken validates the signature and expiry. The expected algorithm is
// pinned: without that check a token signed with "none", or an asymmetric
// algorithm using our secret as a public key, would verify.
func parseToken(secret, raw string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return 0, errInvalidToken
	}

	var id int64
	if _, err := fmt.Sscanf(claims.Subject, "%d", &id); err != nil || id <= 0 {
		return 0, errInvalidToken
	}
	return id, nil
}
