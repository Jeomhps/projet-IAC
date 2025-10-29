package auth

import "github.com/Jeomhps/projet-IAC/api-go/internal/db"

// Package auth provides authentication-related HTTP handlers.
// KISS: define a small handler type and a simple constructor.
// The HTTP methods are implemented in separate files (login.go, me.go).

// Handler wires auth endpoints to the data store and JWT secret.
type Handler struct {
	db        *db.DB
	jwtSecret string
}

// New returns a new auth handler.
func New(d *db.DB, jwtSecret string) *Handler {
	return &Handler{db: d, jwtSecret: jwtSecret}
}
