package users

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// Create adds a new user with a bcrypt-hashed password.
// KISS flow:
// 1) Validate payload
// 2) Reject reserved username "root"
// 3) Insert user (conflict if exists)
// 4) Fetch and return the created user summary
func (h *Handler) Create(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&in); err != nil ||
		strings.TrimSpace(in.Username) == "" ||
		strings.TrimSpace(in.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	if strings.EqualFold(in.Username, "root") {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "Username 'root' is not allowed"})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if _, err := h.db.Exec("INSERT INTO users (username,password_hash,is_admin) VALUES (?,?,?)", in.Username, string(hash), in.IsAdmin); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "User already exists"})
		return
	}

	// Load the newly created user for response shaping
	var u db.User
	_ = h.db.Get(&u, "SELECT * FROM users WHERE username=?", in.Username)

	c.JSON(http.StatusCreated, gin.H{
		"user_id":    u.ID,
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt.UTC().Format(time.RFC3339),
	})
}
