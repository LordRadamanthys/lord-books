package wireless

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// fakeReader plays the device side the way KOReader's calibre plugin does.
type fakeReader struct {
	t        *testing.T
	conn     net.Conn
	r        *bufio.Reader
	received map[string][]byte
	seen     []int
}

func (f *fakeReader) reply(op int, args map[string]any) {
	if err := writeMessage(f.conn, op, args); err != nil {
		f.t.Errorf("client write: %v", err)
	}
}

func (f *fakeReader) run(done chan<- struct{}) {
	defer close(done)
	library := []map[string]any{{"priKey": 0, "uuid": "u-1", "lpath": "Velho - Ana.epub", "extension": "epub"}}
	for {
		m, err := readMessage(f.r)
		if err != nil {
			return
		}
		f.seen = append(f.seen, m.Op)
		switch m.Op {
		case opGetInitializationInfo:
			if _, ok := m.Args["calibre_version"].([]any); !ok {
				f.t.Error("calibre_version must be an array")
			}
			f.reply(opOK, map[string]any{
				"versionOK": true, "maxBookContentPacketLen": 4096, "acceptedExtensions": []string{"epub"},
				"canSendOkToSendbook": true, "canUseCachedMetadata": true, "canAcceptLibraryInfo": true,
				"canStreamBooks": true, "canStreamMetadata": true, "canReceiveBookBinary": true,
				"canDeleteMultipleBooks": true, "deviceName": "Xteink X4", "deviceKind": "X4", "appName": "Test",
			})
		case opGetDeviceInformation:
			f.reply(opOK, map[string]any{"device_info": map[string]any{"device_store_uuid": "dev-1", "device_name": "Xteink X4"}, "version": "1", "device_version": "1"})
		case opSetCalibreDeviceInfo, opSetLibraryInfo:
			f.reply(opOK, map[string]any{})
		case opFreeSpace:
			f.reply(opOK, map[string]any{"free_space_on_device": 1 << 30})
		case opGetBookCount:
			f.reply(opOK, map[string]any{"count": len(library), "willStream": true, "willScan": true})
			for _, b := range library {
				f.reply(opOK, b)
			}
		case opNoop:
			switch {
			case m.Args["ejecting"] == true:
				f.reply(opOK, map[string]any{})
				f.conn.Close()
				return
			case m.Args["count"] != nil:
			case m.Args["priKey"] != nil:
				f.reply(opOK, map[string]any{"title": "Velho", "authors": []string{"Ana"}, "lpath": "Velho - Ana.epub", "size": 10, "uuid": "u-1"})
			default:
				f.reply(opOK, map[string]any{})
			}
		case opSendBook:
			lpath := argString(m.Args, "lpath")
			n := argInt(m.Args, "length")
			f.reply(opOK, map[string]any{})
			data := make([]byte, n)
			if _, err := io.ReadFull(f.r, data); err != nil {
				f.t.Errorf("reading book bytes: %v", err)
				return
			}
			f.received[lpath] = data
		case opSendBooklists, opSendBookMetadata:
			// no reply, as in calibre's protocol
		case opDeleteBook:
			f.reply(opOK, map[string]any{})
			for _, p := range argStrings(m.Args, "lpaths") {
				delete(f.received, p)
				f.reply(opOK, map[string]any{"uuid": "u-" + p})
			}
		case opGetBookFileSegment:
			data, ok := f.received[argString(m.Args, "lpath")]
			if !ok {
				f.reply(opNoop, map[string]any{})
				continue
			}
			f.reply(opOK, map[string]any{"fileLength": len(data)})
			f.conn.Write(data)
		}
	}
}

func TestSessionHandshakeAndSend(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	f := &fakeReader{t: t, conn: clientSide, r: bufio.NewReader(clientSide), received: map[string][]byte{}}
	done := make(chan struct{})
	go f.run(done)

	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	defer ln.Close()
	srv := New("lib-uuid", func() int { return 8080 })
	srv.listener = ln
	ctx, cancel := context.WithCancel(context.Background())
	go srv.handle(ctx, serverSide)

	var sess *session
	for i := 0; i < 100 && sess == nil; i++ {
		time.Sleep(10 * time.Millisecond)
		sess = srv.Session()
	}
	if sess == nil {
		t.Fatalf("device never connected: %+v", srv.Status())
	}
	dev, books := sess.Snapshot()
	if dev.Name != "Xteink X4" || dev.FreeBytes != 1<<30 {
		t.Errorf("device: %+v", dev)
	}
	if len(books) != 1 || books[0].Title != "Velho" || books[0].Path != "Velho - Ana.epub" {
		t.Errorf("books on device: %+v", books)
	}

	src := filepath.Join(t.TempDir(), "b.epub")
	content := []byte("conteudo do livro com mais de um pacote? nao, mas suficiente")
	os.WriteFile(src, content, 0o644)
	res := sess.SendBooks([]Outgoing{{Path: src, Lpath: "Novo - Bia.epub", Metadata: map[string]any{"title": "Novo", "authors": []string{"Bia"}}}})
	if res[0].Err != nil || res[0].Lpath != "Novo - Bia.epub" {
		t.Fatalf("send: %+v", res[0])
	}
	if string(f.received["Novo - Bia.epub"]) != string(content) {
		t.Errorf("device got %q", f.received["Novo - Bia.epub"])
	}
	if _, books = sess.Snapshot(); len(books) != 2 {
		t.Errorf("sent book not recorded: %+v", books)
	}

	var back bytes.Buffer
	if err := sess.FetchBook("Novo - Bia.epub", &back); err != nil || back.String() != string(content) {
		t.Errorf("fetch: err=%v got %q", err, back.String())
	}
	if err := sess.FetchBook("nao-existe.epub", io.Discard); err == nil {
		t.Error("fetching a missing book should fail")
	}
	// A refused fetch must leave the connection usable.
	if err := sess.DeleteBooks([]string{"Novo - Bia.epub"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := f.received["Novo - Bia.epub"]; ok {
		t.Error("device still has the deleted book")
	}
	if _, books = sess.Snapshot(); len(books) != 1 {
		t.Errorf("deleted book still listed: %+v", books)
	}

	cancel() // stopping the server ejects the device
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("device was not ejected")
	}
	if last := f.seen[len(f.seen)-1]; last != opNoop {
		t.Errorf("last message should be the ejecting NOOP, got %d", last)
	}
}

func TestDiscoveryReplyMatchesClients(t *testing.T) {
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	srv := New("lib", func() int { return 8080 })
	srv.port = 9090
	go srv.answerDiscovery(udp)

	c, _ := net.DialUDP("udp4", nil, udp.LocalAddr().(*net.UDPAddr))
	defer c.Close()
	c.Write([]byte("hello"))
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 200)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	// The pattern KOReader uses to parse calibre's reply.
	re := regexp.MustCompile(`^calibre wireless device client \(on (.*)\);(\d+),(\d+)$`)
	m := re.FindStringSubmatch(string(buf[:n]))
	if m == nil || m[2] != "8080" || m[3] != "9090" {
		t.Errorf("reply %q does not match the client pattern", buf[:n])
	}
}

func TestReadMessageRejectsGarbage(t *testing.T) {
	for _, in := range []string{"abc[0,{}]", "99999999999[0]", "5[0,{"} {
		if _, err := readMessage(bufio.NewReader(stringsReader(in))); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

type stringsReader string

func (s stringsReader) Read(p []byte) (int, error) {
	if len(s) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s)
	return n, io.EOF
}
