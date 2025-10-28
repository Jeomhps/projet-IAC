package handlers

import "time"

func formatTimePtr(t *time.Time) any {
	if t == nil { return nil }
	return t.UTC().Format(time.RFC3339)
}
