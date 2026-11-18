// Package freebooks searches and downloads public-domain e-books from Project
// Gutenberg (through the Gutendex catalog API) and Standard Ebooks.
package freebooks

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Gutenberg = "gutenberg"
	Standard  = "standard"

	userAgent    = "LordBooks/0.1 (personal e-book manager)"
	maxBookBytes = 100 << 20
)

type Result struct {
	Source    string   `json:"source"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Authors   []string `json:"authors"`
	Languages []string `json:"languages"`
	CoverURL  string   `json:"cover_url,omitempty"`
	PageURL   string   `json:"page_url"`
	Downloads int      `json:"downloads,omitempty"`
}

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusServiceUnavailable {
			return nil, fmt.Errorf("%s está fora do ar no momento (manutenção)", resp.Request.URL.Host)
		}
		return nil, fmt.Errorf("%s respondeu com erro %d", resp.Request.URL.Host, resp.StatusCode)
	}
	return resp, nil
}

// Search queries one source; lang ("pt", "en"...) only applies to Gutenberg, since
// Standard Ebooks publishes in English only.
func (c *Client) Search(ctx context.Context, source, query, lang string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("digite o que procurar")
	}
	switch source {
	case Gutenberg:
		return c.searchGutenberg(ctx, query, lang)
	case Standard:
		return c.searchStandard(ctx, query)
	}
	return nil, fmt.Errorf("fonte desconhecida: %q", source)
}

const searchTimeout = 20 * time.Second

// searchGutenberg uses Gutendex (richer data: languages, popularity) and falls back to
// Gutenberg's own OPDS search when Gutendex is slow or down.
func (c *Client) searchGutenberg(ctx context.Context, query, lang string) ([]Result, error) {
	res, err := c.searchGutendex(ctx, query, lang)
	if err == nil {
		return res, nil
	}
	res, err2 := c.searchGutenbergOPDS(ctx, query)
	if err2 == nil {
		return res, nil
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout") {
		err = errors.New("o catálogo demorou demais para responder")
	}
	return nil, fmt.Errorf("busca no Gutenberg indisponível agora (%v; site oficial: %v). Tente de novo em alguns minutos", err, err2)
}

func (c *Client) searchGutendex(ctx context.Context, query, lang string) ([]Result, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	params := url.Values{"search": {query}}
	if lang != "" {
		params.Set("languages", lang)
	}
	resp, err := c.get(ctx, "https://gutendex.com/books/?"+params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data struct {
		Results []struct {
			ID      int    `json:"id"`
			Title   string `json:"title"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Languages     []string          `json:"languages"`
			Formats       map[string]string `json:"formats"`
			DownloadCount int               `json:"download_count"`
			MediaType     string            `json:"media_type"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("resposta inválida do Gutenberg: %w", err)
	}
	out := []Result{}
	for _, b := range data.Results {
		if b.MediaType != "" && b.MediaType != "Text" {
			continue // audio books, sheet music...
		}
		if _, ok := b.Formats["application/epub+zip"]; !ok {
			continue
		}
		r := Result{
			Source:    Gutenberg,
			ID:        strconv.Itoa(b.ID),
			Title:     b.Title,
			Languages: b.Languages,
			Downloads: b.DownloadCount,
			PageURL:   fmt.Sprintf("https://www.gutenberg.org/ebooks/%d", b.ID),
			// The mirror keeps covers available when gutenberg.org is down.
			CoverURL: fmt.Sprintf("https://gutenberg.pglaf.org/cache/epub/%d/pg%d.cover.medium.jpg", b.ID, b.ID),
		}
		for _, a := range b.Authors {
			r.Authors = append(r.Authors, gutenbergName(a.Name))
		}
		out = append(out, r)
	}
	return out, nil
}

var (
	gbEntry = regexp.MustCompile(`(?s)<entry>(.*?)</entry>`)
	gbID    = regexp.MustCompile(`/ebooks/(\d+)`)
	gbTitle = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	gbText  = regexp.MustCompile(`(?s)<content[^>]*>(.*?)</content>`)
)

// searchGutenbergOPDS parses gutenberg.org's OPDS search, whose entries carry the
// title, the author as plain content text, and a link to /ebooks/<id>.
func (c *Client) searchGutenbergOPDS(ctx context.Context, query string) ([]Result, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	resp, err := c.get(ctx, "https://www.gutenberg.org/ebooks/search.opds/?"+url.Values{"query": {query}}.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	feed, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	out := []Result{}
	for _, m := range gbEntry.FindAllStringSubmatch(string(feed), -1) {
		id := gbID.FindStringSubmatch(m[1])
		title := gbTitle.FindStringSubmatch(m[1])
		if id == nil || title == nil {
			continue // navigation entries ("Sort by...")
		}
		r := Result{
			Source:   Gutenberg,
			ID:       id[1],
			Title:    html.UnescapeString(strings.TrimSpace(title[1])),
			PageURL:  "https://www.gutenberg.org/ebooks/" + id[1],
			CoverURL: fmt.Sprintf("https://gutenberg.pglaf.org/cache/epub/%s/pg%s.cover.medium.jpg", id[1], id[1]),
		}
		if a := gbText.FindStringSubmatch(m[1]); a != nil {
			if name := html.UnescapeString(strings.TrimSpace(a[1])); name != "" && !strings.Contains(name, "<") {
				r.Authors = []string{name}
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// gutenbergName turns Gutenberg's "Assis, Machado de" into "Machado de Assis".
func gutenbergName(n string) string {
	last, first, ok := strings.Cut(n, ", ")
	if !ok || strings.Contains(first, ",") {
		return n
	}
	return first + " " + last
}

var (
	seItem   = regexp.MustCompile(`(?s)<li typeof="schema:Book" about="(/ebooks/[^"]+)">(.*?)</li>`)
	seTitle  = regexp.MustCompile(`(?s)<a href="[^"]*" property="schema:url"><span property="schema:name">([^<]+)</span>`)
	seAuthor = regexp.MustCompile(`(?s)property="schema:author"[^>]*>.*?<span property="schema:name">([^<]+)</span>`)
	seImage  = regexp.MustCompile(`<img src="([^"]+)"[^>]*property="schema:image"`)
	seEpub   = regexp.MustCompile(`href="(/ebooks/[^"]+/downloads/[^"]+\.epub)"`)
	safeSE   = regexp.MustCompile(`^/ebooks/[a-z0-9_\-]+(/[a-z0-9_\-]+){1,3}$`)
)

func (c *Client) searchStandard(ctx context.Context, query string) ([]Result, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	params := url.Values{"query": {query}, "per-page": {"24"}}
	resp, err := c.get(ctx, "https://standardebooks.org/ebooks?"+params.Encode())
	if err != nil {
		return nil, fmt.Errorf("busca na Standard Ebooks: %w", err)
	}
	defer resp.Body.Close()
	page, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	out := []Result{}
	for _, m := range seItem.FindAllStringSubmatch(string(page), -1) {
		path, body := m[1], m[2]
		if !safeSE.MatchString(path) {
			continue
		}
		r := Result{Source: Standard, ID: path, Languages: []string{"en"}, PageURL: "https://standardebooks.org" + path}
		if t := seTitle.FindStringSubmatch(body); t != nil {
			r.Title = html.UnescapeString(t[1])
		}
		for _, a := range seAuthor.FindAllStringSubmatch(body, -1) {
			r.Authors = append(r.Authors, html.UnescapeString(a[1]))
		}
		if img := seImage.FindStringSubmatch(body); img != nil {
			r.CoverURL = "https://standardebooks.org" + html.UnescapeString(img[1])
		}
		if r.Title != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

// Download saves the book's EPUB to a temp file and returns its path and a file name.
// Only ids that look like the source's own are accepted, so this can't be used to
// fetch arbitrary URLs.
func (c *Client) Download(ctx context.Context, source, id string) (string, string, error) {
	var urls []string
	var name string
	switch source {
	case Gutenberg:
		n, err := strconv.Atoi(id)
		if err != nil || n <= 0 {
			return "", "", errors.New("livro inválido")
		}
		urls = []string{
			fmt.Sprintf("https://www.gutenberg.org/ebooks/%d.epub3.images", n),
			fmt.Sprintf("https://gutenberg.pglaf.org/cache/epub/%d/pg%d-images-3.epub", n, n),
		}
		name = fmt.Sprintf("gutenberg-%d.epub", n)
	case Standard:
		if !safeSE.MatchString(id) {
			return "", "", errors.New("livro inválido")
		}
		resp, err := c.get(ctx, "https://standardebooks.org"+id)
		if err != nil {
			return "", "", err
		}
		page, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			return "", "", err
		}
		for _, m := range seEpub.FindAllStringSubmatch(string(page), -1) {
			href := m[1]
			// The plain EPUB, not the Kobo or "advanced" (EPUB3-only) variants.
			if strings.HasSuffix(href, ".kepub.epub") || strings.HasSuffix(href, "_advanced.epub") {
				continue
			}
			if strings.HasPrefix(href, id+"/downloads/") {
				urls = []string{"https://standardebooks.org" + href + "?source=download"}
				name = href[strings.LastIndex(href, "/")+1:]
				break
			}
		}
		if len(urls) == 0 {
			return "", "", errors.New("não achei o EPUB na página do livro")
		}
	default:
		return "", "", fmt.Errorf("fonte desconhecida: %q", source)
	}

	var lastErr error
	for _, u := range urls {
		path, err := c.save(ctx, u)
		if err == nil {
			return path, name, nil
		}
		lastErr = err
	}
	return "", "", lastErr
}

func (c *Client) save(ctx context.Context, u string) (string, error) {
	resp, err := c.get(ctx, u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.CreateTemp("", "lordbooks-free-*.epub")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBookBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBookBytes {
		err = errors.New("arquivo grande demais")
	}
	if err == nil {
		err = checkEPUB(f.Name())
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// checkEPUB rejects anything that isn't an EPUB (e.g. an HTML error page).
func checkEPUB(path string) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return errors.New("o arquivo baixado não é um EPUB")
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "mimetype" {
			rc, err := f.Open()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(io.LimitReader(rc, 64))
			rc.Close()
			if strings.TrimSpace(string(b)) == "application/epub+zip" {
				return nil
			}
		}
	}
	return errors.New("o arquivo baixado não é um EPUB")
}
