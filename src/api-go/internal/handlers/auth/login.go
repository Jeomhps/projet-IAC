package auth

import (
	"net/http"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Login issues a short-lived access token for valid credentials.
// KISS flow:
// 1) Validate payload
// 2) Load user record
// 3) Check password
// 4) Build JWT and return token response
func (h *Handler) Login(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Username == "" || in.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "username and password are required"})
		return
	}

	var u db.User
	if err := h.db.Get(&u, "SELECT * FROM users WHERE username=?", in.Username); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_grant", "message": "invalid credentials"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_grant", "message": "invalid credentials"})
		return
	}

	// Compact JWT with minimal, useful claims
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": "projet-iac-api-go",
		"sub": u.Username,
		"roles": func() []string {
			if u.IsAdmin {
				return []string{"admin"}
			}
			return []string{}
		}(),
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(60 * time.Minute).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(h.jwtSecret))

	c.JSON(http.StatusOK, gin.H{
		"access_token": signed,
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}
