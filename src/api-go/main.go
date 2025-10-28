package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Jeomhps/projet-IAC/api-go/internal/config"
	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers"
	"github.com/Jeomhps/projet-IAC/api-go/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	d, err := db.OpenFromDATABASE_URL(cfg.DatabaseURL)
	if err != nil { log.Fatalf("db: %v", err) }
	defer d.Close()
	if err := db.EnsureSchema(d); err != nil { log.Fatalf("ensure schema: %v", err) }

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

	authH := handlers.NewAuth(d, cfg.JWTSecret)
	userH := handlers.NewUsers(d)
	machH := handlers.NewMachines(d)
	resH  := handlers.NewReservations(d, cfg.AnsiblePlaybookPath)

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

	addr := os.Getenv("ADDR")
	if addr == "" { addr = ":8080" }
	log.Printf("listening on %s", addr)
	_ = r.Run(addr)
}
