package routes

import (
	"os"

	"github.com/cloudresty/go-env"
	"github.com/gofiber/fiber/v3"
	"resty.dev/v3"
)

// Global configuration variables to share between functions
var (
	apiClient       *resty.Client
	globalAuthToken string
	defaultShareKey = "LofCen6W"
	baseURL         = "https://www.febbox.com"
)

func init() {
	globalAuthToken = env.Get("FEBBOX_UI_COOKIE", "")
	if globalAuthToken == "" {
		globalAuthToken = os.Getenv("FEBBOX_UI_COOKIE")
	}

	apiClient = resty.New()
	apiClient.SetHeaders(map[string]string{
		"x-requested-with": "XMLHttpRequest",
		"user-agent":       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
	})
	if globalAuthToken != "" {
		apiClient.SetHeader("Cookie", "ui="+globalAuthToken)
	}
}

func IndexRoutes(app *fiber.App) {
	// init()
	indexGroup := app.Group("/")

	indexGroup.Get("/", func(c fiber.Ctx) error {
		return c.SendString("API is working!")
	})

	indexGroup.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("Pong!")
	})
}
