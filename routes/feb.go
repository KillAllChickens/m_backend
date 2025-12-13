package routes

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

var filesGroup singleflight.Group

type VideoQuality struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
	Name    string `json:"name"`
	Speed   string `json:"speed"`
	Size    string `json:"size"`
}

type ImdbResponse struct {
	IMDBId string `json:"imdb"`
}

func FebboxAPI(app *fiber.App) {
	febGroup := app.Group("/api/febbox")

	filesCache := cache.New(10*time.Minute, 20*time.Minute)

	client := apiClient
	// defer client.Close()

	// baseURL := "https://www.febbox.com"
	defaultHeaders := map[string]string{
		"x-requested-with": "XMLHttpRequest",
		"user-agent":       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
	}

	// uIToken := env.Get("FEBBOX_UI_COOKIE", "")
	// if uIToken == "" {
	// 	uIToken = os.Getenv("FEBBOX_UI_COOKIE")
	// }

	// log.Println(uIToken)

	defaultShareKey = "LofCen6W"

	client.SetHeaders(defaultHeaders)
	client.SetCookie(&http.Cookie{Name: "ui", Value: globalAuthToken})

	febGroup.Get("/", func(c fiber.Ctx) error {
		return c.SendString("WORKING")
	})
	febGroup.Get("/files", func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json") // Returns JSON
		shareKey := c.Query("shareKey")
		if shareKey == "" {
			shareKey = defaultShareKey
		}
		parentId := c.Query("parentId")

		if cachedList, found := filesCache.Get(shareKey); found {
			return c.JSON(cachedList)
		}

		v, _, _ := filesGroup.Do(shareKey, func() (any, error) {
			resp, err := client.R().Get(baseURL + "/file/file_share_list?share_key=" + shareKey + "&pwd=&parent_id=" + parentId + "&is_html=0")
			if err != nil {
				return nil, err
			}
			var data map[string]any
			if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
				return nil, err
			}

			fileList := data["data"].(map[string]any)["file_list"].([]any)
			filesCache.Set(shareKey, fileList, cache.DefaultExpiration)
			return fileList, nil
		})

		return c.JSON(v)
	})

	febGroup.Get("/links", func(c fiber.Ctx) error {
		fid := c.Query("fid")
		shareKey := c.Query("shareKey")
		if shareKey == "" {
			shareKey = defaultShareKey
		}

		if fid == "" || shareKey == "" {
			return c.Status(400).JSON(fiber.Map{"error": "fid and shareKey are required"})
		}

		resp, err := client.R().
			SetHeader("Referer", baseURL+"/share/"+shareKey).
			Get(baseURL + "/console/video_quality_list?fid=" + fid)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		var data map[string]any
		if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		htmlContent, ok := data["html"].(string)
		if !ok {
			return c.Status(500).SendString("No HTML content found in response")
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
		if err != nil {
			return c.Status(500).SendString("Error parsing HTML content")
		}

		var qualities []VideoQuality

		doc.Find(".file_quality").Each(func(i int, s *goquery.Selection) {
			url, _ := s.Attr("data-url")
			quality, _ := s.Attr("data-quality")
			name := strings.TrimSpace(s.Find(".name").Text())
			speed := strings.TrimSpace(s.Find(".speed span").Text())
			size := strings.TrimSpace(s.Find(".size").Text())

			qualities = append(qualities, VideoQuality{
				URL:     url,
				Quality: quality,
				Name:    name,
				Speed:   speed,
				Size:    size,
			})
		})

		return c.JSON(qualities)
	})

	febGroup.Get("/imdb", func(c fiber.Ctx) error {
		fid := c.Query("fid")
		shareKey := c.Query("shareKey")

		if fid == "" {
			return c.Status(400).JSON(fiber.Map{"error": "at least an fid is required"})
		}

		resp, err := client.R().
			SetHeader("Referer", baseURL+"/share/"+shareKey).
			Get(baseURL + "/console/file_more_info?fid=" + fid)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		if resp.StatusCode() != http.StatusOK {
			return c.Status(resp.StatusCode()).SendString("Febbox returned error status")
		}

		var data map[string]any
		if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
			return c.Status(500).SendString("Error parsing JSON")
		}

		htmlContent, ok := data["html"].(string)
		if !ok {
			return c.JSON(ImdbResponse{IMDBId: ""})
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
		if err != nil {
			return c.Status(500).SendString("Error parsing HTML content")
		}
		imdbID, exists := doc.Find(".imdb").Attr("data-imdb-id")

		if !exists {
			imdbID = ""
		}

		return c.JSON(ImdbResponse{IMDBId: imdbID})
	})

}
