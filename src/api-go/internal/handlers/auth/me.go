package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Me returns a minimal profile for the authenticated user.
// KISS: rely on auth middleware to populate context keys.
func (h *Handler) Me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"username": c.GetString("user"),
		"is_admin": c.GetBool("is_admin"),
	})
}
