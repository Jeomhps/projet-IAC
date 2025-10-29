package main

import (
	"log"
	"net/http"

	"github.com/Jeomhps/projet-IAC/api-go/internal/config"
	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	authh "github.com/Jeomhps/projet-IAC/api-go/internal/handlers/auth"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/machines"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/reservations"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/users"
	"github.com/Jeomhps/projet-IAC/api-go/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	dsn := cfg.DSN()
	d, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer d.Close()

	if err := db.EnsureSchema(d); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	// Seed default admin (optional)
	if cfg.AdminDefaultUser != "" && cfg.AdminDefaultPass != "" {
		if err := db.EnsureDefaultAdmin(d, cfg.AdminDefaultUser, cfg.AdminDefaultPass); err != nil {
			log.Printf("ensure default admin: %v", err)
		}
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	authH := authh.New(d, cfg.JWTSecret)
	userH := users.New(d)
	machH := machines.New(d)

	resH := reservations.NewHandler(d)

	// Public
	r.POST("/auth/login", authH.Login)
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// Authenticated
	auth := r.Group("/")
	auth.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		auth.GET("/auth/me", authH.Me)

		// Users (admin)
		admin := auth.Group("/")
		admin.Use(middleware.RequireAdmin())
		{
			admin.GET("/users", userH.List)
			admin.POST("/users", userH.Create)
			admin.GET("/users/:username", userH.Get)
			admin.PATCH("/users/:username", userH.Update)
			admin.DELETE("/users/:username", userH.Delete)

			admin.POST("/machines", machH.Create)
			admin.PATCH("/machines/:name", machH.Update)
			admin.DELETE("/machines/:name", machH.Delete)
		}

		// Machines
		auth.GET("/machines", machH.List)
		auth.GET("/machines/:name", machH.Get)

		// Reservations
		auth.GET("/reservations", resH.List)
		auth.GET("/reservations/:id", resH.Get)
		auth.POST("/reservations", resH.Create)
		auth.DELETE("/reservations/:id", resH.Delete)
	}

	addr := ":8080"
	log.Printf("listening on %s", addr)
	_ = r.Run(addr)
}
