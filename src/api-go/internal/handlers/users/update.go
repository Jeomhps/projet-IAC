package users

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// Update modifies a user's password and/or admin flag.
// KISS flow:
// 1) Validate payload
// 2) Ensure user exists
// 3) Apply changes (toggle is_admin, update password if non-empty)
// 4) Return the updated user summary
func (h *Handler) Update(c *gin.Context) {
	username := c.Param("username")

	var in struct {
		Password *string `json:"password"`
		IsAdmin  *bool   `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	// Ensure user exists before updating
	var u db.User
	if err := h.db.Get(&u, "SELECT * FROM users WHERE username=?", username); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	// Toggle admin flag if requested
	if in.IsAdmin != nil {
		_, _ = h.db.Exec("UPDATE users SET is_admin=? WHERE username=?", *in.IsAdmin, username)
	}

	// Update password if provided and non-empty
	if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		_, _ = h.db.Exec("UPDATE users SET password_hash=? WHERE username=?", string(hash), username)
	}

	// Load latest user to return
	_ = h.db.Get(&u, "SELECT * FROM users WHERE username=?", username)
	c.JSON(http.StatusOK, gin.H{
		"user_id":    u.ID,
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt.UTC().Format(time.RFC3339),
	})
}
