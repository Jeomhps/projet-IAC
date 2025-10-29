package reservations

import "github.com/Jeomhps/projet-IAC/api-go/internal/db"

// Package reservations provides reservation management HTTP handlers.
// KISS: keep types small, behavior explicit, and files focused.
//
// This file defines the handler type and constructor only.
// The HTTP methods are implemented in dedicated files:
// - list.go:   Handler.List
// - get.go:    Handler.Get
// - create.go: Handler.Create
// - delete.go: Handler.Delete

// Handler wires reservation endpoints to the data store.
type Handler struct{ db *db.DB }

// NewHandler returns a new reservations handler.
func NewHandler(d *db.DB) *Handler { return &Handler{db: d} }
