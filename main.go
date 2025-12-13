package main

import (
	"log"

	"github.com/KillAllChickens/m_backend/routes"
	"github.com/cloudresty/go-env"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

func main() {
	err := env.Load()
	if err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://*.killallchickens.org",
			"http://*.killallchickens.org",
			// firebase URLS
			"https://*.web.app",
			"https://*.firebaseapp.com"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept"},
	}))

	// Mount route groups
	routes.IndexRoutes(app)
	routes.SubtitleRoutes(app)
	routes.FebboxAPI(app)
	routes.StreamingRoutes(app)

	app.Listen(":3000")
}
