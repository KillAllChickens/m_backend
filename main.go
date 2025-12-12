package main

import (
	"github.com/KillAllChickens/m_backend/routes"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

func main() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	app := fiber.New()

	app.Use(cors.New(cors.Config{
        AllowOrigins: []string{"*"},
        AllowHeaders: []string{"Origin", "Content-Type", "Accept"},
    }))


	// Mount route groups
	routes.IndexRoutes(app)
	routes.SubtitleRoutes(app)
	routes.FebboxAPI(app)

	app.Listen(":3000")
}
