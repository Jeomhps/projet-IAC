package users

import (
	"net/http"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
)

// Get returns a single user by username.
// Admin-only; enforced by router middleware.
func (h *Handler) Get(c *gin.Context) {
	username := c.Param("username")

	var u db.User
	if err := h.db.Get(&u, "SELECT * FROM users WHERE username=?", username); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":    u.ID,
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt.UTC().Format(time.RFC3339),
	})
}
