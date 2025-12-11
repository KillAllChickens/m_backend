package routes

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// Subtitle represents the JSON response item
type Subtitle struct {
	Label string `json:"label"`
	Src   string `json:"src"`
}

// SubtitleResponse represents the wrapper JSON
type SubtitleResponse struct {
	Subs  []Subtitle `json:"subs,omitempty"`
	Error string     `json:"error,omitempty"`
}

// ImdbResponse is used to decode the IMDB API
type ImdbResponse struct {
	Imdb string `json:"imdb"`
}

func SubtitleRoutes(app *fiber.App) {
	subsGroup := app.Group("/api/")

	// Endpoint to get subtitles
	subsGroup.Get("/get_subtitles/:fid", func(c fiber.Ctx) error {
		fid := c.Params("fid")
		if fid == "" {
			return c.Status(400).JSON(SubtitleResponse{Error: "Missing FID"})
		}

		// Define paths
		cwd, _ := os.Getwd()
		subsRoot := filepath.Join(cwd, "subtitles")
		fidDir := filepath.Join(subsRoot, fid)

		// 1. Check if files already exist locally
		if _, err := os.Stat(fidDir); err == nil {
			entries, err := os.ReadDir(fidDir)
			if err == nil {
				var existingFiles []string
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".vtt") {
						existingFiles = append(existingFiles, e.Name())
					}
				}

				if len(existingFiles) > 0 {
					sort.Strings(existingFiles)
					var response []Subtitle
					for _, f := range existingFiles {
						labelNum := strings.TrimSuffix(f, ".vtt")
						// Construct URL (Adjust scheme/host as needed or use relative)
						src := fmt.Sprintf("%s/subtitles/%s/%s", c.BaseURL(), fid, f)
						response = append(response, Subtitle{
							Label: fmt.Sprintf("English %s", labelNum),
							Src:   src,
						})
					}
					return c.JSON(SubtitleResponse{Subs: response})
				}
			}
		}

		// 2. Fetch IMDB ID
		imdbURL := fmt.Sprintf("https://feb.superstudies.site/api/febbox/imdb?fid=%s", fid)
		resp, err := http.Get(imdbURL)
		if err != nil || resp.StatusCode != 200 {
			return c.Status(404).JSON(SubtitleResponse{Error: "Failed to fetch IMDB ID"})
		}
		defer resp.Body.Close()

		var imdbData ImdbResponse
		if err := json.NewDecoder(resp.Body).Decode(&imdbData); err != nil || imdbData.Imdb == "" {
			return c.Status(404).JSON(SubtitleResponse{Error: "No IMDB ID found"})
		}

		// 3. Scrape YTS for the list of subtitles
		ytsURL := fmt.Sprintf("https://yts-subs.com/movie-imdb/%s", imdbData.Imdb)
		doc, err := loadHTML(ytsURL)
		if err != nil {
			return c.Status(404).JSON(SubtitleResponse{Error: "Subtitles not found"})
		}

		// Find English links
		var detailLinks []string
		doc.Find("tr").Each(func(i int, s *goquery.Selection) {
			// YTS structure: 2nd TD is language, 5th TD is link
			lang := strings.TrimSpace(s.Find("td").Eq(1).Text())
			if strings.Contains(lang, "English") {
				href, exists := s.Find("td").Eq(4).Find("a").Attr("href")
				if exists {
					detailLinks = append(detailLinks, href)
				}
			}
		})

		if len(detailLinks) == 0 {
			return c.Status(404).JSON(SubtitleResponse{Error: "English subtitles not found"})
		}

		// Limit to top 3
		if len(detailLinks) > 3 {
			detailLinks = detailLinks[:3]
		}

		// Ensure directory exists
		os.MkdirAll(fidDir, 0755)

		// 4. Concurrent Processing
		type ProcessResult struct {
			Index int
			Sub   Subtitle
			Err   error
		}

		// Channel to collect results safely
		resultsChan := make(chan ProcessResult, len(detailLinks))
		var wg sync.WaitGroup

		for i, link := range detailLinks {
			wg.Add(1)

			// Start Goroutine
			go func(idx int, urlPath string) {
				defer wg.Done()

				// Scrape the specific subtitle page
				fullURL := "https://yts-subs.com" + urlPath
				subDoc, err := loadHTML(fullURL)
				if err != nil {
					resultsChan <- ProcessResult{Index: idx, Err: err}
					return
				}

				// Find download button and decode base64 link
				btn := subDoc.Find("a#btn-download-subtitle")
				encodedLink, exists := btn.Attr("data-link")
				if !exists {
					resultsChan <- ProcessResult{Index: idx, Err: fmt.Errorf("no download link")}
					return
				}

				decodedLinkBytes, err := base64.StdEncoding.DecodeString(encodedLink)
				if err != nil {
					resultsChan <- ProcessResult{Index: idx, Err: err}
					return
				}
				zipURL := string(decodedLinkBytes)

				// Download ZIP
				zipResp, err := http.Get(zipURL)
				if err != nil {
					resultsChan <- ProcessResult{Index: idx, Err: err}
					return
				}
				defer zipResp.Body.Close()

				bodyBytes, _ := io.ReadAll(zipResp.Body)

				// Open ZIP
				zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
				if err != nil {
					resultsChan <- ProcessResult{Index: idx, Err: err}
					return
				}

				// Find .srt file
				var srtContent []byte
				for _, zf := range zipReader.File {
					if strings.HasSuffix(strings.ToLower(zf.Name), ".srt") {
						rc, err := zf.Open()
						if err == nil {
							srtContent, _ = io.ReadAll(rc)
							rc.Close()
							break
						}
					}
				}

				if srtContent == nil {
					resultsChan <- ProcessResult{Index: idx, Err: fmt.Errorf("no srt in zip")}
					return
				}

				// Convert to VTT
				vttContent := srtToVtt(srtContent)

				// Save file
				fileName := fmt.Sprintf("%d.vtt", idx+1)
				savePath := filepath.Join(fidDir, fileName)
				err = os.WriteFile(savePath, vttContent, 0644)
				if err != nil {
					resultsChan <- ProcessResult{Index: idx, Err: err}
					return
				}

				// Success result
				src := fmt.Sprintf("%s/subtitles/%s/%s", c.BaseURL(), fid, fileName)
				resultsChan <- ProcessResult{
					Index: idx,
					Sub: Subtitle{
						Label: fmt.Sprintf("English %d", idx+1),
						Src:   src,
					},
				}

			}(i, link)
		}

		wg.Wait()
		close(resultsChan)

		// Collect results
		var finalSubs []Subtitle
		// Create a map to ensure we return them in order 1, 2, 3 even if they finish out of order
		resultsMap := make(map[int]Subtitle)

		for res := range resultsChan {
			if res.Err == nil {
				resultsMap[res.Index] = res.Sub
			} else {
				fmt.Printf("Error processing sub %d: %v\n", res.Index, res.Err)
			}
		}

		// Rebuild slice in order
		for i := 0; i < len(detailLinks); i++ {
			if sub, ok := resultsMap[i]; ok {
				finalSubs = append(finalSubs, sub)
			}
		}

		if len(finalSubs) == 0 {
			return c.Status(500).JSON(SubtitleResponse{Error: "Failed to process subtitles"})
		}

		return c.JSON(SubtitleResponse{Subs: finalSubs})
	})

	// Serve the static files for subtitles
	// Equivalent to: @app.route('/subtitles/<fid>/<path:filename>')

	// app.Static("/subtitles", "./subtitles")
	app.Use("/subtitles", static.New("./subtitles"))
}

// Helper to load HTML
func loadHTML(url string) (*goquery.Document, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}
	return goquery.NewDocumentFromReader(res.Body)
}

// Helper to convert SRT bytes to VTT bytes
func srtToVtt(srt []byte) []byte {
	// Simple decoding strategy (Go defaults to UTF-8, handling Latin-1 manually is complex
	// without external libs, but YTS is usually UTF-8).
	// We sanitize basic string issues.

	content := string(srt)
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	lines := strings.Split(content, "\n")
	var vttBuilder strings.Builder

	vttBuilder.WriteString("WEBVTT\n\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 1. Skip numeric identifiers (e.g. just "1" or "25")
		if isNumeric(line) {
			continue
		}

		// 2. Fix timestamps: 00:00:01,000 --> 00:00:04,000 becomes ... .000
		if strings.Contains(line, "-->") {
			line = strings.ReplaceAll(line, ",", ".")
		}

		vttBuilder.WriteString(line + "\n")
	}

	return []byte(vttBuilder.String())
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
