package users

import (
	"net/http"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
)

// List returns all users with minimal fields.
// Admin-only; enforced by router middleware.
func (h *Handler) List(c *gin.Context) {
	var users []db.User
	if err := h.db.Select(&users, "SELECT * FROM users ORDER BY id ASC"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"user_id":    u.ID,
			"username":   u.Username,
			"is_admin":   u.IsAdmin,
			"created_at": u.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, out)
}
