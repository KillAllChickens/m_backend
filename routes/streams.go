package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
)

func getRealVideoLink(fid, shareKey string) (string, error) {
	if shareKey == "" {
		shareKey = defaultShareKey // Uses global defaultShareKey
	}

	targetURL := fmt.Sprintf("https://feb.superstudies.site/api/febbox/links?fid=%s&shareKey=%s", fid, shareKey)

	resp, err := apiClient.R().Get(targetURL) // Uses global apiClient
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode())
	}

	var data []struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	if len(data) == 0 {
		return "", errors.New("no video links returned")
	}

	bestURL := data[0].URL
	if bestURL == "" {
		return "", errors.New("url field is empty")
	}

	return bestURL, nil
}

func StreamingRoutes(app *fiber.App) {
	streamGroup := app.Group("/stream")

	streamGroup.Get("/proxy/:fid", func(c fiber.Ctx) error {
		fid := c.Params("fid")
		shareKey := c.Query("shareKey")

		realVideoURL, err := getRealVideoLink(fid, shareKey)
		if err != nil {
			return c.Status(500).SendString("Failed to extract video link: " + err.Error())
		}

		req, err := http.NewRequest("GET", realVideoURL, nil)
		if err != nil {
			return c.Status(500).SendString("Failed to create upstream request")
		}

		clientRange := c.Get("Range")
		if clientRange != "" {
			req.Header.Set("Range", clientRange)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

		upstreamClient := &http.Client{

			Timeout: 30 * time.Second,
		}

		resp, err := upstreamClient.Do(req)
		if err != nil {
			return c.Status(502).SendString("Upstream connection failed: " + err.Error())
		}

		c.Set("Content-Type", resp.Header.Get("Content-Type"))
		c.Set("Content-Length", resp.Header.Get("Content-Length"))
		c.Set("Content-Range", resp.Header.Get("Content-Range"))
		c.Set("Accept-Ranges", "bytes")

		return c.Status(resp.StatusCode).SendStream(resp.Body)
	})
}
