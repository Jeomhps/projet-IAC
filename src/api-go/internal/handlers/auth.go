package handlers

import (
	"net/http"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Auth struct {
	db        *db.DB
	jwtSecret string
}

func NewAuth(d *db.DB, secret string) *Auth { return &Auth{db: d, jwtSecret: secret} }

func (h *Auth) Login(c *gin.Context) {
	var in struct{ Username, Password string }
	if err := c.ShouldBindJSON(&in); err != nil || in.Username == "" || in.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request","message":"username and password are required"}); return
	}
	var u db.User
	if err := h.db.Get(&u, "SELECT * FROM users WHERE username=?", in.Username); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error":"invalid_grant","message":"invalid credentials"}); return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error":"invalid_grant","message":"invalid credentials"}); return
	}
	claims := jwt.MapClaims{
		"iss": "projet-iac-api-go",
		"sub": u.Username,
		"roles": func() []string { if u.IsAdmin { return []string{"admin"} } ; return []string{} }(),
		"iat": time.Now().Unix(),
		"nbf": time.Now().Unix(),
		"exp": time.Now().Add(60 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok, _ := token.SignedString([]byte(h.jwtSecret))
	c.JSON(http.StatusOK, gin.H{
		"access_token": tok,
		"token_type": "Bearer",
		"expires_in": 3600,
	})
}

func (h *Auth) Me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"username": c.GetString("user"),
		"is_admin": c.GetBool("is_admin"),
	})
}
