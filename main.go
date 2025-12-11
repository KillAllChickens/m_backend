package main

import (
	"github.com/KillAllChickens/m_backend/routes"
	"github.com/gofiber/fiber/v3"
)

func main() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	app := fiber.New()

	// Mount route groups
	routes.IndexRoutes(app)
	routes.SubtitleRoutes(app)
	routes.FebboxAPI(app)

	app.Listen(":3000")
}
