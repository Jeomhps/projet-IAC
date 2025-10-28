package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Missing Bearer token"})
			return
		}
		tokenStr := strings.TrimSpace(auth[len("Bearer "):])
		claims := jwt.MapClaims{}
		tok, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !tok.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Invalid token"})
			return
		}
		sub, _ := claims["sub"].(string)
		isAdmin := false
		if arr, ok := claims["roles"].([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok && s == "admin" { isAdmin = true; break }
			}
		}
		if sub == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "Invalid subject"})
			return
		}
		c.Set("user", sub)
		c.Set("is_admin", isAdmin)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !c.GetBool("is_admin") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "Missing required role: admin"})
			return
		}
		c.Next()
	}
}

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		user := c.GetString("user")
		if user == "" { user = "-" }
		log.Printf("%s %s -> %d in %dms user=%s",
			c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start).Milliseconds(), user)
	}
}
