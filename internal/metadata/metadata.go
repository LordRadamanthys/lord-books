package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Candidate struct {
	Source      string   `json:"source"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	ISBN        string   `json:"isbn,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	PubDate     string   `json:"pub_date,omitempty"`
	Language    string   `json:"language,omitempty"`
	Description string   `json:"description,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
}

type Client struct {
	http      *http.Client
	googleKey string
}

func New(googleKey string) *Client {
	return &Client{http: &http.Client{Timeout: 12 * time.Second}, googleKey: googleKey}
}

type provider struct {
	name   string
	search func(context.Context, string, string) ([]Candidate, error)
}

// Search queries all providers concurrently; a failing provider becomes a warning, not an error.
func (c *Client) Search(ctx context.Context, query, isbn string) ([]Candidate, []string) {
	providers := []provider{{"Google Books", c.googleBooks}, {"OpenLibrary", c.openLibrary}}
	results := make([][]Candidate, len(providers))
	warnings := make([]string, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := p.search(ctx, query, isbn)
			if err != nil {
				warnings[i] = p.name + ": " + err.Error()
				return
			}
			results[i] = res
		}()
	}
	wg.Wait()

	var out []Candidate

	// Interleave so the top hit of each provider shows first.
	for i := 0; ; i++ {
		added := false
		for _, r := range results {
			if i < len(r) {
				out = append(out, r[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	var warns []string
	for _, w := range warnings {
		if w != "" {
			warns = append(warns, w)
		}
	}
	return out, warns
}

type statusError struct{ code int }

func (e statusError) Error() string {
	switch e.code {
	case 429:
		return "limite de consultas excedido"
	case 403:
		return "acesso negado (verifique a chave de API)"
	}
	return fmt.Sprintf("erro HTTP %d", e.code)
}

func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "LordBooks/0.1")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return statusError{resp.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

func (c *Client) googleBooks(ctx context.Context, query, isbn string) ([]Candidate, error) {
	q := query
	if isbn != "" {
		q = "isbn:" + isbn
	}
	params := url.Values{"maxResults": {"8"}, "printType": {"books"}, "q": {q}}
	if c.googleKey != "" {
		params.Set("key", c.googleKey)
	}
	u := "https://www.googleapis.com/books/v1/volumes?" + params.Encode()

	var resp struct {
		Items []struct {
			VolumeInfo struct {
				Title               string   `json:"title"`
				Subtitle            string   `json:"subtitle"`
				Authors             []string `json:"authors"`
				Publisher           string   `json:"publisher"`
				PublishedDate       string   `json:"publishedDate"`
				Description         string   `json:"description"`
				Language            string   `json:"language"`
				PageCount           int      `json:"pageCount"`
				IndustryIdentifiers []struct {
					Type       string `json:"type"`
					Identifier string `json:"identifier"`
				} `json:"industryIdentifiers"`
				ImageLinks struct {
					Thumbnail string `json:"thumbnail"`
				} `json:"imageLinks"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return nil, err
	}

	var out []Candidate
	for _, it := range resp.Items {
		v := it.VolumeInfo
		cand := Candidate{
			Source:      "Google Books",
			Title:       v.Title,
			Authors:     v.Authors,
			Publisher:   v.Publisher,
			PubDate:     v.PublishedDate,
			Language:    v.Language,
			Description: v.Description,
			PageCount:   v.PageCount,
		}
		for _, id := range v.IndustryIdentifiers {
			if id.Type == "ISBN_13" || (id.Type == "ISBN_10" && cand.ISBN == "") {
				cand.ISBN = id.Identifier
			}
		}
		if t := v.ImageLinks.Thumbnail; t != "" {
			t = strings.Replace(t, "http://", "https://", 1)
			t = strings.Replace(t, "&edge=curl", "", 1)
			cand.CoverURL = t
		}
		out = append(out, cand)
	}
	return out, nil
}

var olLangs = map[string]string{
	"por": "pt", "eng": "en", "spa": "es", "fre": "fr", "fra": "fr",
	"ger": "de", "deu": "de", "ita": "it", "jpn": "ja", "rus": "ru",
}

func (c *Client) openLibrary(ctx context.Context, query, isbn string) ([]Candidate, error) {
	// The search API matches ISBNs at the work level (often another language's edition),
	// so an ISBN lookup reads the exact edition record instead.
	if isbn != "" {
		if cand, err := c.olEdition(ctx, isbn); err == nil {
			return []Candidate{*cand}, nil
		}
	}

	params := url.Values{}
	if isbn != "" {
		params.Set("isbn", isbn)
	} else {
		params.Set("q", query)
	}
	params.Set("limit", "8")
	params.Set("fields", "key,title,author_name,isbn,publisher,first_publish_year,language,cover_i,number_of_pages_median")
	u := "https://openlibrary.org/search.json?" + params.Encode()

	var resp struct {
		Docs []struct {
			Key         string   `json:"key"`
			Title       string   `json:"title"`
			AuthorName  []string `json:"author_name"`
			ISBN        []string `json:"isbn"`
			Publisher   []string `json:"publisher"`
			FirstYear   int      `json:"first_publish_year"`
			Language    []string `json:"language"`
			CoverID     int      `json:"cover_i"`
			NumberPages int      `json:"number_of_pages_median"`
		} `json:"docs"`
	}
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return nil, err
	}

	var out []Candidate
	for _, d := range resp.Docs {
		cand := Candidate{
			Source:    "OpenLibrary",
			Title:     d.Title,
			Authors:   d.AuthorName,
			PageCount: d.NumberPages,
		}
		if isbn != "" {
			cand.ISBN = isbn
		} else {
			cand.ISBN = pickISBN(d.ISBN)
		}
		if len(d.Publisher) > 0 {
			cand.Publisher = d.Publisher[0]
		}
		if d.FirstYear > 0 {
			cand.PubDate = strconv.Itoa(d.FirstYear)
		}
		if len(d.Language) == 1 {
			if l, ok := olLangs[d.Language[0]]; ok {
				cand.Language = l
			}
		}
		if d.CoverID > 0 {
			cand.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", d.CoverID)
		}
		out = append(out, cand)
	}

	// Search results carry no description; fetch it from the work record for the top hits.
	var wg sync.WaitGroup
	for i := range out {
		if i >= 4 || !strings.HasPrefix(resp.Docs[i].Key, "/works/") {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w, err := c.olWork(ctx, resp.Docs[i].Key); err == nil {
				out[i].Description = w.description()
			}
		}()
	}
	wg.Wait()
	return out, nil
}

type olWorkRecord struct {
	Description json.RawMessage `json:"description"`
	Authors     []struct {
		Author struct {
			Key string `json:"key"`
		} `json:"author"`
	} `json:"authors"`
}

// description is either a plain string or {"type": ..., "value": ...}.
func (w *olWorkRecord) description() string {
	if len(w.Description) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(w.Description, &s) == nil {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	json.Unmarshal(w.Description, &obj)
	return obj.Value
}

func (c *Client) olWork(ctx context.Context, key string) (*olWorkRecord, error) {
	var w olWorkRecord
	if err := c.getJSON(ctx, "https://openlibrary.org"+key+".json", &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (c *Client) olEdition(ctx context.Context, isbn string) (*Candidate, error) {
	var ed struct {
		Title         string   `json:"title"`
		Publishers    []string `json:"publishers"`
		PublishDate   string   `json:"publish_date"`
		Covers        []int    `json:"covers"`
		NumberOfPages int      `json:"number_of_pages"`
		Languages     []struct {
			Key string `json:"key"`
		} `json:"languages"`
		Authors []struct {
			Key string `json:"key"`
		} `json:"authors"`
		Works []struct {
			Key string `json:"key"`
		} `json:"works"`
	}
	if err := c.getJSON(ctx, "https://openlibrary.org/isbn/"+url.PathEscape(isbn)+".json", &ed); err != nil {
		return nil, err
	}
	if ed.Title == "" {
		return nil, fmt.Errorf("edition without title")
	}

	cand := &Candidate{
		Source:    "OpenLibrary",
		Title:     ed.Title,
		ISBN:      isbn,
		PubDate:   ed.PublishDate,
		PageCount: ed.NumberOfPages,
	}
	if len(ed.Publishers) > 0 {
		cand.Publisher = ed.Publishers[0]
	}
	if len(ed.Languages) > 0 {
		cand.Language = olLangs[strings.TrimPrefix(ed.Languages[0].Key, "/languages/")]
	}
	for _, id := range ed.Covers {
		if id > 0 {
			cand.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", id)
			break
		}
	}

	authorKeys := make([]string, 0, len(ed.Authors))
	for _, a := range ed.Authors {
		authorKeys = append(authorKeys, a.Key)
	}
	if len(ed.Works) > 0 {
		if w, err := c.olWork(ctx, ed.Works[0].Key); err == nil {
			cand.Description = w.description()
			if len(authorKeys) == 0 {
				for _, a := range w.Authors {
					authorKeys = append(authorKeys, a.Author.Key)
				}
			}
		}
	}
	for _, key := range authorKeys {
		var a struct {
			Name string `json:"name"`
		}
		if c.getJSON(ctx, "https://openlibrary.org"+key+".json", &a) == nil && a.Name != "" {
			cand.Authors = append(cand.Authors, a.Name)
		}
	}
	return cand, nil
}

func pickISBN(isbns []string) string {
	for _, s := range isbns {
		if len(s) == 13 {
			return s
		}
	}
	if len(isbns) > 0 {
		return isbns[0]
	}
	return ""
}

func allowedCoverHost(u *url.URL) bool {
	if u.Scheme != "https" {
		return false
	}
	h := u.Hostname()
	switch h {
	case "books.google.com", "books.googleusercontent.com", "covers.openlibrary.org", "archive.org":
		return true
	}
	// OpenLibrary covers redirect to archive.org storage nodes.
	return strings.HasSuffix(h, ".archive.org")
}

// DownloadCover fetches a cover only from the known provider hosts, so the
// apply endpoint can't be used to make the server fetch arbitrary URLs.
func (c *Client) DownloadCover(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !allowedCoverHost(u) {
		return nil, "", fmt.Errorf("cover url not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "LordBooks/0.1")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("cover: status %d", resp.StatusCode)
	}
	if !allowedCoverHost(resp.Request.URL) {
		return nil, "", fmt.Errorf("cover redirected to disallowed host")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	ext := ".jpg"
	switch http.DetectContentType(data) {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
	case "image/webp":
		ext = ".webp"
	default:
		return nil, "", fmt.Errorf("cover is not an image")
	}
	// OpenLibrary returns a 1x1 gif/tiny image when a cover is missing.
	if len(data) < 1000 {
		return nil, "", fmt.Errorf("cover too small")
	}
	return data, ext, nil
}
