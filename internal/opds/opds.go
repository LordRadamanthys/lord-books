// Package opds serves the library as an OPDS 1.2 catalog, so readers with an OPDS
// client (Boox, KOReader, Moon+ Reader, ...) can browse and download books over Wi-Fi.
// It is read-only and runs on its own port, separate from the management UI.
package opds

import (
	"context"
	"crypto/subtle"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LordRadamanthys/lord-books/internal/db"
)

const (
	pageSize    = 50
	navType     = "application/atom+xml;profile=opds-catalog;kind=navigation"
	acqType     = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	relAcquire  = "http://opds-spec.org/acquisition"
	relImage    = "http://opds-spec.org/image"
	relThumb    = "http://opds-spec.org/image/thumbnail"
	catalogName = "Lord Books"
)

var mimeTypes = map[string]string{
	"epub": "application/epub+zip",
	"pdf":  "application/pdf",
	"azw3": "application/vnd.amazon.ebook",
	"mobi": "application/x-mobipocket-ebook",
}

// Server runs the catalog on demand.
type Server struct {
	store *db.DB

	mu       sync.Mutex
	http     *http.Server
	port     int
	password string
}

func New(store *db.DB) *Server { return &Server{store: store} }

type Status struct {
	Running   bool     `json:"running"`
	Port      int      `json:"port"`
	URLs      []string `json:"urls"`
	Protected bool     `json:"protected"`
}

// Start listens on every interface; password may be empty for an open catalog.
func (s *Server) Start(port int, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.http != nil {
		return nil
	}
	ln, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return fmt.Errorf("não foi possível abrir a porta %d: %w", port, err)
	}
	s.port, s.password = port, password
	s.http = &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func(h *http.Server) {
		if err := h.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("opds: %v", err)
		}
	}(s.http)
	log.Printf("opds: catalog on port %d", port)
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	h := s.http
	s.http = nil
	s.mu.Unlock()
	if h != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		h.Shutdown(ctx)
	}
}

func (s *Server) Status(addrs []string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Running: s.http != nil, Port: s.port, Protected: s.password != ""}
	if st.Running {
		for _, a := range addrs {
			st.URLs = append(st.URLs, fmt.Sprintf("http://%s:%d/opds", a, s.port))
		}
	}
	return st
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/opds", http.StatusFound)
	})
	mux.HandleFunc("GET /opds", s.root)
	mux.HandleFunc("GET /opds/books", s.allBooks)
	mux.HandleFunc("GET /opds/search", s.search)
	mux.HandleFunc("GET /opds/search.xml", s.openSearch)
	mux.HandleFunc("GET /opds/authors", s.authors)
	mux.HandleFunc("GET /opds/authors/{id}", s.byAuthor)
	mux.HandleFunc("GET /opds/series", s.series)
	mux.HandleFunc("GET /opds/series/{id}", s.bySeries)
	mux.HandleFunc("GET /opds/tags", s.tags)
	mux.HandleFunc("GET /opds/tags/{id}", s.byTag)
	mux.HandleFunc("GET /opds/books/{id}/file", s.file)
	mux.HandleFunc("GET /opds/books/{id}/cover", s.cover)
	return logRequests(s.auth(mux))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests records each catalog request; readers rarely explain their errors, so
// this is the way to see what a device asked for and what it got.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		_, _, hasAuth := r.BasicAuth()
		log.Printf("opds: %s %s %s -> %d (auth=%v, agent=%q)", r.RemoteAddr, r.Method, r.URL.RequestURI(), rec.status, hasAuth, r.UserAgent())
	})
}

// auth applies HTTP Basic auth when a password is set; any user name is accepted,
// since reader apps differ in which field they let people fill in.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		pw := s.password
		s.mu.Unlock()
		if pw != "" {
			_, got, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(pw)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Lord Books", charset="UTF-8"`)
				http.Error(w, "senha necessária", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Atom model ───

type feed struct {
	XMLName      xml.Name `xml:"feed"`
	Xmlns        string   `xml:"xmlns,attr"`
	XmlnsDC      string   `xml:"xmlns:dc,attr"`
	XmlnsOPDS    string   `xml:"xmlns:opds,attr"`
	XmlnsOS      string   `xml:"xmlns:opensearch,attr"`
	ID           string   `xml:"id"`
	Title        string   `xml:"title"`
	Updated      string   `xml:"updated"`
	Author       person   `xml:"author"`
	TotalResults int      `xml:"opensearch:totalResults,omitempty"`
	ItemsPerPage int      `xml:"opensearch:itemsPerPage,omitempty"`
	Links        []link   `xml:"link"`
	Entries      []entry  `xml:"entry"`
}

type person struct {
	Name string `xml:"name"`
}

type link struct {
	Rel   string `xml:"rel,attr,omitempty"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Title string `xml:"title,attr,omitempty"`
}

type text struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type category struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr"`
}

type entry struct {
	Title      string     `xml:"title"`
	ID         string     `xml:"id"`
	Updated    string     `xml:"updated"`
	Authors    []person   `xml:"author,omitempty"`
	Language   string     `xml:"dc:language,omitempty"`
	Publisher  string     `xml:"dc:publisher,omitempty"`
	Issued     string     `xml:"dc:issued,omitempty"`
	Identifier string     `xml:"dc:identifier,omitempty"`
	Categories []category `xml:"category,omitempty"`
	Summary    *text      `xml:"summary,omitempty"`
	Content    *text      `xml:"content,omitempty"`
	Links      []link     `xml:"link"`
}

func newFeed(id, title, self, kind string) *feed {
	return &feed{
		Xmlns:     "http://www.w3.org/2005/Atom",
		XmlnsDC:   "http://purl.org/dc/terms/",
		XmlnsOPDS: "http://opds-spec.org/2010/catalog",
		XmlnsOS:   "http://a9.com/-/spec/opensearch/1.1/",
		ID:        "urn:lordbooks:" + id,
		Title:     title,
		Updated:   time.Now().UTC().Format(time.RFC3339),
		Author:    person{Name: catalogName},
		Links: []link{
			{Rel: "self", Href: self, Type: kind},
			{Rel: "start", Href: "/opds", Type: navType},
			{Rel: "up", Href: "/opds", Type: navType},
			{Rel: "search", Href: "/opds/search.xml", Type: "application/opensearchdescription+xml"},
		},
	}
}

func writeFeed(w http.ResponseWriter, f *feed, kind string) {
	w.Header().Set("Content-Type", kind+";charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(f); err != nil {
		log.Printf("opds: encode: %v", err)
	}
}

func navEntry(id, title, summary, href string) entry {
	return entry{
		Title:   title,
		ID:      "urn:lordbooks:" + id,
		Updated: time.Now().UTC().Format(time.RFC3339),
		Content: &text{Type: "text", Value: summary},
		Links:   []link{{Rel: "subsection", Href: href, Type: acqType}},
	}
}

// ─── Feeds ───

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	f := newFeed("root", catalogName, "/opds", navType)
	stats, _ := s.store.Stats(r.Context())
	f.Entries = []entry{
		navEntry("recent", "Recentes", fmt.Sprintf("%d livros, os adicionados por último primeiro", stats.Total), "/opds/books?sort=added"),
		navEntry("title", "Por título", "Todos os livros em ordem alfabética", "/opds/books?sort=title"),
		navEntry("authors", "Autores", "Livros agrupados por autor", "/opds/authors"),
		navEntry("series", "Séries", "Livros agrupados por série", "/opds/series"),
		navEntry("tags", "Tags", "Livros agrupados por tag", "/opds/tags"),
	}
	for i := range f.Entries[2:] {
		f.Entries[2+i].Links[0].Type = navType
	}
	writeFeed(w, f, navType)
}

func (s *Server) allBooks(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	title := "Recentes"
	if sort == "title" {
		title = "Por título"
	} else {
		sort = "added"
	}
	s.bookFeed(w, r, "books-"+sort, title, db.ListFilter{Sort: sort})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	s.bookFeed(w, r, "search", "Busca: "+q, db.ListFilter{Query: q, Sort: "title"})
}

func (s *Server) openSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/opensearchdescription+xml;charset=utf-8")
	fmt.Fprint(w, xml.Header+`<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Lord Books</ShortName>
  <Description>Buscar livros por título ou autor</Description>
  <InputEncoding>UTF-8</InputEncoding>
  <OutputEncoding>UTF-8</OutputEncoding>
  <Url type="`+acqType+`" template="/opds/search?q={searchTerms}"/>
</OpenSearchDescription>`)
}

func (s *Server) byAuthor(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.bookFeed(w, r, "author-"+strconv.Itoa(id), s.nameOf(r, "authors", id), db.ListFilter{AuthorID: id, Sort: "title"})
}

func (s *Server) bySeries(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.bookFeed(w, r, "series-"+strconv.Itoa(id), s.nameOf(r, "series", id), db.ListFilter{SeriesID: id, Sort: "series"})
}

func (s *Server) byTag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.bookFeed(w, r, "tag-"+strconv.Itoa(id), s.nameOf(r, "tags", id), db.ListFilter{TagID: id, Sort: "title"})
}

func (s *Server) nameOf(r *http.Request, kind string, id int) string {
	var names map[int]string
	switch kind {
	case "authors":
		list, _ := s.store.ListAuthors(r.Context())
		names = map[int]string{}
		for _, a := range list {
			names[a.ID] = a.Name
		}
	case "series":
		list, _ := s.store.ListSeries(r.Context())
		names = map[int]string{}
		for _, a := range list {
			names[a.ID] = a.Name
		}
	case "tags":
		list, _ := s.store.ListTags(r.Context())
		names = map[int]string{}
		for _, a := range list {
			names[a.ID] = a.Name
		}
	}
	if n := names[id]; n != "" {
		return n
	}
	return catalogName
}

func (s *Server) authors(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListAuthors(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	f := newFeed("authors", "Autores", "/opds/authors", navType)
	for _, a := range list {
		f.Entries = append(f.Entries, navEntry(fmt.Sprintf("author-%d", a.ID), a.Name, "", fmt.Sprintf("/opds/authors/%d", a.ID)))
	}
	writeFeed(w, f, navType)
}

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSeries(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	f := newFeed("series", "Séries", "/opds/series", navType)
	for _, a := range list {
		f.Entries = append(f.Entries, navEntry(fmt.Sprintf("series-%d", a.ID), a.Name, "", fmt.Sprintf("/opds/series/%d", a.ID)))
	}
	writeFeed(w, f, navType)
}

func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListTags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	f := newFeed("tags", "Tags", "/opds/tags", navType)
	for _, t := range list {
		if t.Count == 0 {
			continue
		}
		f.Entries = append(f.Entries, navEntry(fmt.Sprintf("tag-%d", t.ID), t.Name,
			fmt.Sprintf("%d livro(s)", t.Count), fmt.Sprintf("/opds/tags/%d", t.ID)))
	}
	writeFeed(w, f, navType)
}

func (s *Server) bookFeed(w http.ResponseWriter, r *http.Request, id, title string, filter db.ListFilter) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	filter.Limit = pageSize
	filter.Offset = (page - 1) * pageSize
	books, total, err := s.store.ListBooks(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	self := r.URL.Path
	q := r.URL.Query()
	pageURL := func(p int) string {
		q.Set("page", strconv.Itoa(p))
		return self + "?" + q.Encode()
	}
	f := newFeed(id, title, pageURL(page), acqType)
	f.TotalResults, f.ItemsPerPage = total, pageSize
	if page > 1 {
		f.Links = append(f.Links, link{Rel: "previous", Href: pageURL(page - 1), Type: acqType})
	}
	if page*pageSize < total {
		f.Links = append(f.Links, link{Rel: "next", Href: pageURL(page + 1), Type: acqType})
	}
	for _, b := range books {
		f.Entries = append(f.Entries, bookEntry(b))
	}
	writeFeed(w, f, acqType)
}

func bookEntry(b db.Book) entry {
	e := entry{
		Title:     b.Title,
		ID:        fmt.Sprintf("urn:lordbooks:book:%d", b.ID),
		Updated:   atomTime(b.DateAdded),
		Language:  b.Language,
		Publisher: b.Publisher,
		Issued:    b.PubDate,
	}
	if len(e.Issued) > 10 {
		e.Issued = e.Issued[:10]
	}
	if b.ISBN != "" {
		e.Identifier = "urn:isbn:" + b.ISBN
	}
	for _, a := range b.Authors {
		e.Authors = append(e.Authors, person{Name: a})
	}
	for _, t := range b.Tags {
		e.Categories = append(e.Categories, category{Term: t, Label: t})
	}
	summary := plainText(b.Description)
	if b.SeriesName != "" {
		summary = strings.TrimSpace(fmt.Sprintf("Série: %s #%g\n\n%s", b.SeriesName, b.SeriesIndex, summary))
	}
	if summary != "" {
		e.Summary = &text{Type: "text", Value: summary}
	}
	mt := mimeTypes[b.Format]
	if mt == "" {
		mt = "application/octet-stream"
	}
	// The file name in the URL lets readers that ignore Content-Disposition save it sensibly.
	fileName := url.PathEscape(safeName(b.Title) + "." + b.Format)
	e.Links = append(e.Links, link{Rel: relAcquire, Href: fmt.Sprintf("/opds/books/%d/file?name=%s", b.ID, fileName), Type: mt, Title: strings.ToUpper(b.Format)})
	if b.HasCover {
		cover := fmt.Sprintf("/opds/books/%d/cover", b.ID)
		e.Links = append(e.Links, link{Rel: relImage, Href: cover, Type: "image/jpeg"}, link{Rel: relThumb, Href: cover, Type: "image/jpeg"})
	}
	return e
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.GetBook(r.Context(), id)
	if err != nil || b.FilePath == "" {
		http.NotFound(w, r)
		return
	}
	if mt := mimeTypes[b.Format]; mt != "" {
		w.Header().Set("Content-Type", mt)
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": safeName(b.Title) + "." + b.Format}))
	http.ServeFile(w, r, b.FilePath)
}

func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.GetBook(r.Context(), id)
	if err != nil || b.CoverPath == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, b.CoverPath)
}

func atomTime(s string) string {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
	if s = strings.TrimSpace(s); s == "" {
		return "livro"
	}
	return s
}
