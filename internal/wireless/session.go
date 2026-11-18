package wireless

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type session struct {
	mu      sync.Mutex // one request/response exchange at a time
	conn    net.Conn
	r       *bufio.Reader
	libUUID string
	closed  bool

	device        Device
	maxPacket     int
	canSendOK     bool // client answers SEND_BOOK before receiving the bytes
	useCache      bool
	acceptLibInfo bool
	books         []BookOnDevice // as last reported by the device
}

// BookOnDevice is a book the reader reported having received from calibre.
type BookOnDevice struct {
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
	Path    string   `json:"path"` // calibre "lpath", relative to the reader's inbox
	Size    int64    `json:"size"`
	Format  string   `json:"format"`
	UUID    string   `json:"-"`
}

func (s *session) send(op int, args map[string]any) error {
	s.conn.SetWriteDeadline(time.Now().Add(replyTimeout))
	return writeMessage(s.conn, op, args)
}

func (s *session) receive() (message, error) {
	s.conn.SetReadDeadline(time.Now().Add(replyTimeout))
	return s.receiveNoDeadline()
}

func (s *session) receiveNoDeadline() (message, error) {
	return readMessage(s.r)
}

func (s *session) call(op int, args map[string]any) (message, error) {
	if err := s.send(op, args); err != nil {
		return message{}, err
	}
	return s.receive()
}

func (s *session) callOK(op int, args map[string]any, what string) (map[string]any, error) {
	m, err := s.call(op, args)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if m.Op == opError {
		return nil, fmt.Errorf("%s: %s", what, argString(m.Args, "message"))
	}
	if m.Op != opOK {
		return nil, fmt.Errorf("%s: resposta inesperada (opcode %d)", what, m.Op)
	}
	return m.Args, nil
}

// handshake runs the same opening sequence calibre does when a device connects.
func (s *session) handshake() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, err := s.callOK(opGetInitializationInfo, map[string]any{
		"serverProtocolVersion":  1,
		"validExtensions":        []string{"epub", "pdf", "azw3", "mobi", "kepub", "txt"},
		"passwordChallenge":      "",
		"currentLibraryName":     libraryName,
		"currentLibraryUUID":     s.libUUID,
		"pubdateFormat":          "MMM yyyy",
		"timestampFormat":        "dd MMM yyyy",
		"lastModifiedFormat":     "dd MMM yyyy",
		"calibre_version":        calibreVersion,
		"canSupportUpdateBooks":  true,
		"canSupportLpathChanges": true,
	}, "inicialização")
	if err != nil {
		return err
	}
	if !argBool(info, "versionOK") {
		return errors.New("o aparelho recusou a versão do protocolo")
	}
	s.maxPacket = int(argInt(info, "maxBookContentPacketLen"))
	if s.maxPacket <= 0 {
		return errors.New("o aparelho informou um tamanho de pacote inválido")
	}
	if s.maxPacket > 1<<20 {
		s.maxPacket = 1 << 20
	}
	s.canSendOK = argBool(info, "canSendOkToSendbook")
	s.useCache = argBool(info, "canUseCachedMetadata")
	s.acceptLibInfo = argBool(info, "canAcceptLibraryInfo")
	s.device = Device{
		Name:       argString(info, "deviceName"),
		Kind:       argString(info, "deviceKind"),
		App:        argString(info, "appName"),
		Address:    s.conn.RemoteAddr().String(),
		Extensions: argStrings(info, "acceptedExtensions"),
	}
	if s.device.Name == "" {
		s.device.Name = s.device.Kind
	}
	if s.device.Name == "" {
		s.device.Name = "Aparelho sem fio"
	}
	if host, _, err := net.SplitHostPort(s.device.Address); err == nil {
		s.device.Address = host
	}

	devInfo, err := s.callOK(opGetDeviceInformation, map[string]any{}, "informações do aparelho")
	if err != nil {
		return err
	}
	drive, _ := devInfo["device_info"].(map[string]any)
	if drive == nil {
		drive = map[string]any{}
	}
	drive["location_code"] = "main"
	drive["prefix"] = ""
	drive["last_library_uuid"] = s.libUUID
	drive["calibre_version"] = "9.18.0"
	drive["date_last_connected"] = time.Now().UTC().Format("2006-01-02T15:04:05+00:00")
	if _, err := s.callOK(opSetCalibreDeviceInfo, drive, "registro do aparelho"); err != nil {
		return err
	}

	if s.acceptLibInfo {
		if _, err := s.callOK(opSetLibraryInfo, map[string]any{
			"libraryName":   libraryName,
			"libraryUuid":   s.libUUID,
			"fieldMetadata": map[string]any{},
			"otherInfo":     map[string]any{"id_link_rules": map[string]any{}},
		}, "informações da biblioteca"); err != nil {
			return err
		}
	}

	if err := s.refreshFreeSpace(); err != nil {
		return err
	}
	if err := s.loadBooks(); err != nil {
		return err
	}
	// calibre always follows the book list with its own booklist; nothing to update.
	return s.send(opSendBooklists, map[string]any{
		"count": 0, "collections": map[string]any{}, "willStreamMetadata": true, "supportsSync": false,
	})
}

func (s *session) refreshFreeSpace() error {
	res, err := s.callOK(opFreeSpace, map[string]any{}, "espaço livre")
	if err != nil {
		return err
	}
	s.device.FreeBytes = argInt(res, "free_space_on_device")
	return nil
}

// loadBooks asks the device for the books it holds, exactly as calibre's books() does.
func (s *session) loadBooks() error {
	res, err := s.callOK(opGetBookCount, map[string]any{
		"canStream":                true,
		"canScan":                  true,
		"willUseCachedMetadata":    s.useCache,
		"supportsSync":             false,
		"canSupportBookFormatSync": true,
	}, "lista de livros")
	if err != nil {
		return err
	}
	count := int(argInt(res, "count"))
	entries := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		m, err := s.receive()
		if err != nil {
			return fmt.Errorf("lista de livros: %w", err)
		}
		entries = append(entries, m.Args)
	}

	if s.useCache {
		// Entries are short cache keys; request the full metadata of every book.
		if err := s.send(opNoop, map[string]any{"count": len(entries)}); err != nil {
			return err
		}
		for _, e := range entries {
			if err := s.send(opNoop, map[string]any{"priKey": e["priKey"]}); err != nil {
				return err
			}
		}
		full := make([]map[string]any, 0, len(entries))
		for range entries {
			m, err := s.receive()
			if err != nil {
				return fmt.Errorf("metadados dos livros: %w", err)
			}
			full = append(full, m.Args)
		}
		entries = full
	}

	s.books = s.books[:0]
	for _, e := range entries {
		lpath := argString(e, "lpath")
		b := BookOnDevice{
			Title:   argString(e, "title"),
			Authors: argStrings(e, "authors"),
			Path:    lpath,
			Size:    argInt(e, "size"),
			UUID:    argString(e, "uuid"),
		}
		if i := strings.LastIndex(lpath, "."); i >= 0 {
			b.Format = strings.ToLower(lpath[i+1:])
		}
		// Some readers keep little metadata; Lord Books and calibre name files
		// "Title - Author.ext", so fall back to that.
		if b.Title == "" || len(b.Authors) == 0 {
			t, a := titleAuthorsFromLpath(lpath)
			if b.Title == "" {
				b.Title = t
			}
			if len(b.Authors) == 0 {
				b.Authors = a
			}
		}
		s.books = append(s.books, b)
	}
	return nil
}

// keepAlive pings the device until it goes away or the server stops, mirroring
// calibre's periodic NOOP; readers drop idle connections otherwise.
func (s *session) keepAlive(ctx context.Context) {
	t := time.NewTicker(keepAliveEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.eject()
			return
		case <-t.C:
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				return
			}
			_, err := s.callOK(opNoop, map[string]any{}, "keep-alive")
			if err != nil {
				s.closeLocked()
			}
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
		}
	}
}

func (s *session) closeLocked() {
	if !s.closed {
		s.closed = true
		s.conn.Close()
	}
}

// eject tells the device calibre is disconnecting, then closes the socket.
func (s *session) eject() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.call(opNoop, map[string]any{"ejecting": true})
	s.closeLocked()
}

// Disconnect tells the device the session is over, like calibre's eject.
func (s *session) Disconnect() { s.eject() }

// Snapshot returns the device info and its books.
func (s *session) Snapshot() (Device, []BookOnDevice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.device, append([]BookOnDevice(nil), s.books...)
}

// Refresh re-reads free space and the book list from the device.
func (s *session) Refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errNotConnected
	}
	if err := s.refreshFreeSpace(); err != nil {
		s.closeLocked()
		return err
	}
	if err := s.loadBooks(); err != nil {
		s.closeLocked()
		return err
	}
	return nil
}

// DeleteBooks asks the reader to delete books, as calibre's delete_books does: the
// reader acknowledges the request, then answers once per book.
func (s *session) DeleteBooks(lpaths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errNotConnected
	}
	if _, err := s.callOK(opDeleteBook, map[string]any{"lpaths": lpaths}, "apagar livros"); err != nil {
		s.closeLocked()
		return err
	}
	for range lpaths {
		if _, err := s.receive(); err != nil {
			s.closeLocked()
			return fmt.Errorf("apagar livros: %w", err)
		}
	}
	gone := map[string]bool{}
	for _, p := range lpaths {
		gone[p] = true
	}
	kept := s.books[:0]
	for _, b := range s.books {
		if !gone[b.Path] {
			kept = append(kept, b)
		}
	}
	s.books = kept
	s.refreshFreeSpace()
	return nil
}

// FetchBook copies a book from the reader into w (calibre's get_file).
func (s *session) FetchBook(lpath string, w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errNotConnected
	}
	m, err := s.call(opGetBookFileSegment, map[string]any{
		"lpath": lpath, "position": 0, "thisBook": 0, "totalBooks": 1,
		"canStream": true, "canStreamBinary": true,
	})
	if err != nil {
		s.closeLocked()
		return err
	}
	if m.Op != opOK {
		return &peerError{"o aparelho não enviou o arquivo"}
	}
	n := argInt(m.Args, "fileLength")
	if n <= 0 {
		return &peerError{"o aparelho informou um arquivo vazio"}
	}
	s.conn.SetReadDeadline(time.Now().Add(replyTimeout + time.Duration(n/20000)*time.Second))
	if _, err := io.CopyN(w, s.r, n); err != nil {
		// The stream is out of sync once a transfer breaks off.
		s.closeLocked()
		return err
	}
	return nil
}

// Outgoing is one book to transfer.
type Outgoing struct {
	Path     string // local file to send
	Lpath    string // name on the device
	Metadata map[string]any
}

// SendResult reports one transfer; Lpath is where the device actually stored it.
type SendResult struct {
	Lpath string
	Err   error
}

// SendBooks transfers files the way calibre's upload_books + sync_booklists do.
// It stops at the first connection error, since the stream is then unusable.
func (s *session) SendBooks(items []Outgoing) []SendResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := make([]SendResult, len(items))
	if s.closed {
		for i := range results {
			results[i].Err = errNotConnected
		}
		return results
	}

	var sent []map[string]any
	for i, it := range items {
		lpath, err := s.sendOne(it, i, len(items))
		results[i] = SendResult{Lpath: lpath, Err: err}
		if err != nil {
			var pe *peerError
			if !errors.As(err, &pe) { // connection broken: fail the rest
				s.closeLocked()
				for j := i + 1; j < len(items); j++ {
					results[j].Err = errNotConnected
				}
				return results
			}
			continue
		}
		it.Metadata["lpath"] = lpath
		sent = append(sent, it.Metadata)
		s.books = append(s.books, BookOnDevice{
			Title: argString(it.Metadata, "title"), Authors: argStrings(it.Metadata, "authors"),
			Path: lpath, Size: argInt(it.Metadata, "size"), Format: formatOf(lpath),
		})
	}

	if len(sent) > 0 {
		if err := s.send(opSendBooklists, map[string]any{
			"count": len(sent), "collections": map[string]any{}, "willStreamMetadata": true, "supportsSync": false,
		}); err == nil {
			for i, md := range sent {
				if s.send(opSendBookMetadata, map[string]any{
					"index": i, "count": len(sent), "data": md, "supportsSync": false,
				}) != nil {
					break
				}
			}
		}
		s.refreshFreeSpace()
	}
	return results
}

// peerError is a refusal reported by the device; the connection itself is still fine.
type peerError struct{ msg string }

func (e *peerError) Error() string { return e.msg }

func (s *session) sendOne(it Outgoing, index, total int) (string, error) {
	f, err := os.Open(it.Path)
	if err != nil {
		return "", &peerError{err.Error()}
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", &peerError{err.Error()}
	}
	it.Metadata["size"] = st.Size()
	it.Metadata["lpath"] = it.Lpath

	if err := s.send(opSendBook, map[string]any{
		"lpath":                  it.Lpath,
		"length":                 st.Size(),
		"metadata":               it.Metadata,
		"thisBook":               index,
		"totalBooks":             total,
		"willStreamBooks":        true,
		"willStreamBinary":       true,
		"wantsSendOkToSendbook":  s.canSendOK,
		"canSupportLpathChanges": true,
	}); err != nil {
		return "", err
	}
	lpath := it.Lpath
	if s.canSendOK {
		m, err := s.receive()
		if err != nil {
			return "", err
		}
		if m.Op == opError {
			return "", &peerError{"o aparelho recusou: " + argString(m.Args, "message")}
		}
		if lp := argString(m.Args, "lpath"); lp != "" {
			lpath = lp
		}
	}

	// A large book on a slow reader can take a while; allow time proportional to size.
	s.conn.SetWriteDeadline(time.Now().Add(replyTimeout + time.Duration(st.Size()/20000)*time.Second))
	buf := make([]byte, s.maxPacket)
	if _, err := io.CopyBuffer(writerOnly{s.conn}, f, buf); err != nil {
		return "", err
	}
	return lpath, nil
}

// writerOnly hides net.Conn's ReadFrom so CopyBuffer really uses packet-sized writes.
type writerOnly struct{ io.Writer }

func titleAuthorsFromLpath(lpath string) (string, []string) {
	base := lpath
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	i := strings.LastIndex(base, " - ")
	if i <= 0 {
		return strings.TrimSpace(base), nil
	}
	var authors []string
	for _, a := range strings.Split(base[i+3:], ",") {
		if a = strings.TrimSpace(a); a != "" {
			authors = append(authors, a)
		}
	}
	return strings.TrimSpace(base[:i]), authors
}

func formatOf(lpath string) string {
	if i := strings.LastIndex(lpath, "."); i >= 0 {
		return strings.ToLower(lpath[i+1:])
	}
	return ""
}
