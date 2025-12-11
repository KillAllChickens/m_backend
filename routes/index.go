package routes

import "github.com/gofiber/fiber/v3"

func IndexRoutes(app *fiber.App) {
    userGroup := app.Group("/")

    userGroup.Get("/", func(c fiber.Ctx) error {
        return c.JSON(fiber.Map{"message": "list of users"})
    })

    userGroup.Get("/ping", func(c fiber.Ctx) error {
        return c.SendString("Pong!")
    })
}
