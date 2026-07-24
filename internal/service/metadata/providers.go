package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jjones/e-biblioteca/internal/models"
)

type LookupResult struct {
	Source      string
	Title       string
	Subtitle    string
	Description string
	Publisher   string
	Published   string
	ISBN10      string
	ISBN13      string
	Authors     []string
	Categories  []string
	CoverURL    string
	PageCount   int
	Language    string
}

var httpClient = &http.Client{
	Timeout: 12 * time.Second,
	// Covers must not follow redirects to internal hosts.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if err := validateCoverURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	},
}

// Allowed cover hosts from known metadata providers.
var coverHostAllowlist = map[string]bool{
	"covers.openlibrary.org":      true,
	"books.google.com":            true,
	"books.googleusercontent.com": true,
	"lh3.googleusercontent.com":   true,
}

func validateCoverURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid cover url")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("cover url must be https")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("empty cover host")
	}
	if coverHostAllowlist[host] {
		return nil
	}
	// Allow google books CDN subdomains.
	if strings.HasSuffix(host, ".googleusercontent.com") ||
		strings.HasSuffix(host, ".googleapis.com") ||
		host == "www.googleapis.com" {
		return nil
	}
	return fmt.Errorf("cover host not allowed: %s", host)
}

func Lookup(ctx context.Context, title, author, isbn string) ([]LookupResult, error) {
	var results []LookupResult
	if isbn != "" {
		if r, err := openLibraryISBN(ctx, isbn); err == nil && r != nil {
			results = append(results, *r)
		}
	}
	if title != "" {
		if more, err := openLibrarySearch(ctx, title, author); err == nil {
			results = append(results, more...)
		}
		if more, err := googleBooksSearch(ctx, title, author, isbn); err == nil {
			results = append(results, more...)
		}
	}
	// de-dupe by title+source
	seen := map[string]bool{}
	var out []LookupResult
	for _, r := range results {
		key := r.Source + "|" + strings.ToLower(r.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out, nil
}

func openLibraryISBN(ctx context.Context, isbn string) (*LookupResult, error) {
	isbn = onlyDigits(isbn)
	u := fmt.Sprintf("https://openlibrary.org/isbn/%s.json", url.PathEscape(isbn))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "e-biblioteca/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var data struct {
		Title         string   `json:"title"`
		Subtitle      string   `json:"subtitle"`
		Publishers    []string `json:"publishers"`
		PublishDate   string   `json:"publish_date"`
		NumberOfPages int      `json:"number_of_pages"`
		Covers        []int    `json:"covers"`
		ISBN10        []string `json:"isbn_10"`
		ISBN13        []string `json:"isbn_13"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	r := &LookupResult{
		Source:    "Open Library",
		Title:     data.Title,
		Subtitle:  data.Subtitle,
		Published: data.PublishDate,
		PageCount: data.NumberOfPages,
	}
	if len(data.Publishers) > 0 {
		r.Publisher = data.Publishers[0]
	}
	if len(data.ISBN10) > 0 {
		r.ISBN10 = data.ISBN10[0]
	}
	if len(data.ISBN13) > 0 {
		r.ISBN13 = data.ISBN13[0]
	}
	if len(data.Covers) > 0 {
		r.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", data.Covers[0])
	}
	return r, nil
}

func openLibrarySearch(ctx context.Context, title, author string) ([]LookupResult, error) {
	q := url.Values{}
	q.Set("title", title)
	if author != "" {
		q.Set("author", author)
	}
	q.Set("limit", "5")
	u := "https://openlibrary.org/search.json?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "e-biblioteca/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var data struct {
		Docs []struct {
			Title        string   `json:"title"`
			Subtitle     string   `json:"subtitle"`
			AuthorName   []string `json:"author_name"`
			Publisher    []string `json:"publisher"`
			FirstPublish int      `json:"first_publish_year"`
			ISBN         []string `json:"isbn"`
			CoverI       int      `json:"cover_i"`
			Language     []string `json:"language"`
			Subject      []string `json:"subject"`
		} `json:"docs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	var out []LookupResult
	for _, d := range data.Docs {
		r := LookupResult{
			Source:     "Open Library",
			Title:      d.Title,
			Subtitle:   d.Subtitle,
			Authors:    d.AuthorName,
			Categories: firstN(d.Subject, 5),
		}
		if len(d.Publisher) > 0 {
			r.Publisher = d.Publisher[0]
		}
		if d.FirstPublish > 0 {
			r.Published = fmt.Sprintf("%d-01-01", d.FirstPublish)
		}
		for _, isbn := range d.ISBN {
			digits := onlyDigits(isbn)
			if len(digits) == 13 && r.ISBN13 == "" {
				r.ISBN13 = digits
			}
			if len(digits) == 10 && r.ISBN10 == "" {
				r.ISBN10 = digits
			}
		}
		if d.CoverI > 0 {
			r.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", d.CoverI)
		}
		if len(d.Language) > 0 {
			r.Language = d.Language[0]
		}
		out = append(out, r)
	}
	return out, nil
}

func googleBooksSearch(ctx context.Context, title, author, isbn string) ([]LookupResult, error) {
	qparts := []string{}
	if isbn != "" {
		qparts = append(qparts, "isbn:"+onlyDigits(isbn))
	} else {
		if title != "" {
			qparts = append(qparts, "intitle:"+title)
		}
		if author != "" {
			qparts = append(qparts, "inauthor:"+author)
		}
	}
	q := url.Values{}
	q.Set("q", strings.Join(qparts, "+"))
	q.Set("maxResults", "5")
	u := "https://www.googleapis.com/books/v1/volumes?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "e-biblioteca/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var data struct {
		Items []struct {
			VolumeInfo struct {
				Title               string   `json:"title"`
				Subtitle            string   `json:"subtitle"`
				Authors             []string `json:"authors"`
				Publisher           string   `json:"publisher"`
				PublishedDate       string   `json:"publishedDate"`
				Description         string   `json:"description"`
				PageCount           int      `json:"pageCount"`
				Categories          []string `json:"categories"`
				Language            string   `json:"language"`
				IndustryIdentifiers []struct {
					Type       string `json:"type"`
					Identifier string `json:"identifier"`
				} `json:"industryIdentifiers"`
				ImageLinks struct {
					Thumbnail string `json:"thumbnail"`
					Small     string `json:"smallThumbnail"`
				} `json:"imageLinks"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	var out []LookupResult
	for _, item := range data.Items {
		v := item.VolumeInfo
		r := LookupResult{
			Source:      "Google Books",
			Title:       v.Title,
			Subtitle:    v.Subtitle,
			Description: v.Description,
			Publisher:   v.Publisher,
			Published:   v.PublishedDate,
			Authors:     v.Authors,
			Categories:  v.Categories,
			PageCount:   v.PageCount,
			Language:    v.Language,
			CoverURL:    v.ImageLinks.Thumbnail,
		}
		if r.CoverURL == "" {
			r.CoverURL = v.ImageLinks.Small
		}
		r.CoverURL = upgradeToHTTPS(r.CoverURL)
		for _, id := range v.IndustryIdentifiers {
			if id.Type == "ISBN_13" {
				r.ISBN13 = id.Identifier
			}
			if id.Type == "ISBN_10" {
				r.ISBN10 = id.Identifier
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func firstN(ss []string, n int) []string {
	if len(ss) <= n {
		return ss
	}
	return ss[:n]
}

// upgradeToHTTPS rewrites only the URL scheme from http to https.
func upgradeToHTTPS(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return raw
	}
	u.Scheme = "https"
	return u.String()
}

func DownloadCover(ctx context.Context, coverURL string) ([]byte, string, error) {
	if coverURL == "" {
		return nil, "", fmt.Errorf("empty url")
	}
	// Normalize common http covers to https before allowlist check.
	coverURL = upgradeToHTTPS(coverURL)
	if err := validateCoverURL(coverURL); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "e-biblioteca/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	}
	return data, ext, nil
}

func ApplyLookup(meta *models.ExtractedMetadata, r LookupResult) {
	if r.Title != "" {
		meta.Title = r.Title
	}
	if r.Subtitle != "" {
		meta.Subtitle = r.Subtitle
	}
	if r.Description != "" {
		meta.Description = r.Description
	}
	if r.Publisher != "" {
		meta.Publisher = r.Publisher
	}
	if r.Published != "" {
		meta.PublishedDate = r.Published
	}
	if r.ISBN10 != "" {
		meta.ISBN10 = r.ISBN10
	}
	if r.ISBN13 != "" {
		meta.ISBN13 = r.ISBN13
	}
	if len(r.Authors) > 0 {
		meta.Authors = r.Authors
	}
	if len(r.Categories) > 0 {
		meta.Categories = r.Categories
	}
	if r.PageCount > 0 {
		meta.PageCount = r.PageCount
	}
	if r.Language != "" {
		meta.Language = r.Language
	}
}
