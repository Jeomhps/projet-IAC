package machines

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Delete removes a machine by name.
// KISS behavior:
// - If the machine is reserved, reject with a clear message.
// - If the machine doesn't exist, return not_found.
// - Otherwise, delete and return a simple confirmation.
func (h *Handler) Delete(c *gin.Context) {
	name := c.Param("name")

	// Ensure the machine exists and is not reserved
	var reserved bool
	if err := h.db.Get(&reserved, "SELECT reserved FROM machines WHERE name=?", name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	if reserved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Machine is currently reserved; release first"})
		return
	}

	// Delete the machine
	_, _ = h.db.Exec("DELETE FROM machines WHERE name=?", name)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
