package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type Users struct{ db *db.DB }
func NewUsers(d *db.DB) *Users { return &Users{db: d} }

func (h *Users) List(c *gin.Context) {
	var users []db.User
	if err := h.db.Select(&users, "SELECT * FROM users ORDER BY id ASC"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"server_error"}); return
	}
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"user_id": u.ID, "username": u.Username, "is_admin": u.IsAdmin, "created_at": u.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Users) Create(c *gin.Context) {
	var in struct{ Username, Password string; IsAdmin bool }
	if err := c.ShouldBindJSON(&in); err != nil || in.Username == "" || in.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request"}); return
	}
	if strings.EqualFold(in.Username, "root") {
		c.JSON(http.StatusConflict, gin.H{"error":"conflict","message":"Username 'root' is not allowed"}); return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if _, err := h.db.Exec("INSERT INTO users (username,password_hash,is_admin) VALUES (?,?,?)", in.Username, string(hash), in.IsAdmin); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error":"conflict","message":"User already exists"}); return
	}
	var u db.User
	_ = h.db.Get(&u, "SELECT * FROM users WHERE username=?", in.Username)
	c.JSON(http.StatusCreated, gin.H{"user_id": u.ID, "username": u.Username, "is_admin": u.IsAdmin, "created_at": u.CreatedAt.UTC().Format(time.RFC3339)})
}

func (h *Users) Get(c *gin.Context) {
	username := c.Param("username")
	var u db.User
	if err := h.db.Get(&u, "SELECT * FROM users WHERE username=?", username); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error":"not_found"}); return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": u.ID, "username": u.Username, "is_admin": u.IsAdmin, "created_at": u.CreatedAt.UTC().Format(time.RFC3339)})
}

func (h *Users) Update(c *gin.Context) {
	username := c.Param("username")
	var in struct{ Password *string `json:"password"`; IsAdmin *bool `json:"is_admin"` }
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request"}); return
	}
	var u db.User
	if err := h.db.Get(&u, "SELECT * FROM users WHERE username=?", username); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error":"not_found"}); return
	}
	if in.IsAdmin != nil {
		_, _ = h.db.Exec("UPDATE users SET is_admin=? WHERE username=?", *in.IsAdmin, username)
	}
	if in.Password != nil && *in.Password != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		_, _ = h.db.Exec("UPDATE users SET password_hash=? WHERE username=?", string(hash), username)
	}
	_ = h.db.Get(&u, "SELECT * FROM users WHERE username=?", username)
	c.JSON(http.StatusOK, gin.H{"user_id": u.ID, "username": u.Username, "is_admin": u.IsAdmin, "created_at": u.CreatedAt.UTC().Format(time.RFC3339)})
}

func (h *Users) Delete(c *gin.Context) {
	username := c.Param("username")
	_, err := h.db.Exec("DELETE FROM users WHERE username=?", username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error":"not_found"}); return
	}
	c.JSON(http.StatusOK, gin.H{"message":"deleted"})
}
