package machines

import (
	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
)

// Package machines provides machine management HTTP handlers.
// KISS: keep types small, behavior explicit, and files focused.
//
// This file defines the handler type and constructor only.
// The HTTP methods are split into dedicated, focused files:
// - list.go:   Handler.List
// - get.go:    Handler.Get
// - create.go: Handler.Create
// - update.go: Handler.Update
// - delete.go: Handler.Delete

// Handler wires machine endpoints to the data store.
type Handler struct{ db *db.DB }

// New returns a new machines handler.
func New(d *db.DB) *Handler { return &Handler{db: d} }
