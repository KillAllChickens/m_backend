package main

import (
	"log"

	"github.com/KillAllChickens/m_backend/routes"
	"github.com/gofiber/fiber/v3"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load() // 👈 load .env file
	if err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	// Mount route groups
	routes.IndexRoutes(app)
	routes.SubtitleRoutes(app)
	routes.FebboxAPI(app)

	app.Listen(":3000")
}
