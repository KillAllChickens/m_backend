package routes

import (
	// "net/http"
	// "github.com/PuerkitoBio/goquery"

	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/patrickmn/go-cache"
	"resty.dev/v3"

	"github.com/cloudresty/go-env"
)

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

	linkCache := cache.New(10*time.Minute, 20*time.Minute)

	client := resty.New()
	// defer client.Close()

	baseURL := "https://www.febbox.com"
	defaultHeaders := map[string]string{
		"x-requested-with": "XMLHttpRequest",
		"user-agent":       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
	}
	// uIToken := env.Get("FEBBOX_UI_COOKIE", "")
	_ = env.Load()

	// 2. Get the token safely
	// We try the library first, then fall back to standard os.Getenv to ensure
	// it works with Docker -e flags or system exports.
	uIToken := env.Get("FEBBOX_UI_COOKIE", "")
	if uIToken == "" {
		uIToken = os.Getenv("FEBBOX_UI_COOKIE")
	}

	fmt.Println(uIToken)

	client.SetHeaders(defaultHeaders)
	client.SetCookie(&http.Cookie{Name: "ui", Value: uIToken})

	febGroup.Get("/", func(c fiber.Ctx) error {
		return c.SendString("WORKING")
	})
	febGroup.Get("/files", func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json") // Returns JSON
		shareKey := c.Query("shareKey")
		parentId := c.Query("parentId")

		resp, err := client.R().Get(baseURL + "/file/file_share_list?share_key=" + shareKey + "&pwd=&parent_id=" + parentId + "&is_html=0")
		if err != nil {
			return err
		}
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
			return err
		}

		fileList := data["data"].(map[string]interface{})["file_list"].([]interface{})

		return c.JSON(fileList)
	})

	// async getLinks(shareKey, fid, cookie = null) {
	//        const url = `${this.baseUrl}/console/video_quality_list?fid=${fid}`;
	//        this._setReferer(shareKey);

	//        const data = await this._fetchJson(url, cookie);
	//        const htmlResponse = data.html;

	//        // Parse HTML response and extract file qualities
	//        const dom = new JSDOM(htmlResponse);
	//        const doc = dom.window.document;
	//        // return doc;
	//        return this._extractFileQualities(doc);
	//    }
	febGroup.Get("/links", func(c fiber.Ctx) error {
		fid := c.Query("fid")
		shareKey := c.Query("shareKey") // shareKey is required for the Referer header

		if fid == "" || shareKey == "" {
			return c.Status(400).JSON(fiber.Map{"error": "fid and shareKey are required"})
		}

		if cachedLinks, found := linkCache.Get(fid); found {
			return c.JSON(cachedLinks)
		}

		resp, err := client.R().
			SetHeader("Referer", baseURL+"/share/"+shareKey).
			Get(baseURL + "/console/video_quality_list?fid=" + fid)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		// 2. Unmarshal the JSON response
		// return c.Send(resp.Bytes())
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		// 3. Extract the HTML string
		htmlContent, ok := data["html"].(string)
		if !ok {
			return c.Status(500).SendString("No HTML content found in response")
		}

		// 4. Parse the HTML using Goquery (replaces jsdom)
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
		if err != nil {
			return c.Status(500).SendString("Error parsing HTML content")
		}

		var qualities []VideoQuality

		// 5. Extract data from .file_quality elements
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

		if len(qualities) > 0 {
			linkCache.Set(fid, qualities, cache.DefaultExpiration)
		}

		return c.JSON(qualities)
	})

	febGroup.Get("/imdb", func(c fiber.Ctx) error {
		fid := c.Query("fid")
		shareKey := c.Query("shareKey")

		if fid == "" {
			return c.Status(400).JSON(fiber.Map{"error": "at least an fid is required"})
		}

		// 1. Request data from Febbox
		// Note: The endpoint is /console/file_more_info
		resp, err := client.R().
			SetHeader("Referer", baseURL+"/share/"+shareKey).
			Get(baseURL + "/console/file_more_info?fid=" + fid)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		if resp.StatusCode() != http.StatusOK {
			return c.Status(resp.StatusCode()).SendString("Febbox returned error status")
		}

		// 2. Parse JSON response to get HTML string
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
			return c.Status(500).SendString("Error parsing JSON")
		}

		htmlContent, ok := data["html"].(string)
		if !ok {
			// If no HTML is returned, it usually means no info was found
			return c.JSON(ImdbResponse{IMDBId: ""})
		}

		// 3. Parse HTML using GoQuery
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
		if err != nil {
			return c.Status(500).SendString("Error parsing HTML content")
		}

		// 4. Find .imdb element and extract data-imdb-id
		// JS: doc.querySelector(".imdb").getAttribute("data-imdb-id")
		imdbID, exists := doc.Find(".imdb").Attr("data-imdb-id")

		if !exists {
			imdbID = ""
		}

		return c.JSON(ImdbResponse{IMDBId: imdbID})
	})

}
