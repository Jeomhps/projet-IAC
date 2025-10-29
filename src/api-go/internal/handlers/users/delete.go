package users

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Delete removes a user by username.
// KISS flow:
// 1) Attempt deletion
// 2) If no rows affected -> not_found
// 3) Otherwise -> return a simple confirmation
func (h *Handler) Delete(c *gin.Context) {
	username := c.Param("username")

	res, err := h.db.Exec("DELETE FROM users WHERE username=?", username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
