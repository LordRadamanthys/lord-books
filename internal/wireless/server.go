package wireless

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort    = 9090
	replyTimeout   = 60 * time.Second
	keepAliveEvery = 10 * time.Second
	libraryName    = "Lord Books"
)

// calibreVersion is what we report as the "calibre" version. Clients gate features on
// it (KOReader reports send errors only to calibre >= 4.18), so it must be modern.
var calibreVersion = []int{9, 18, 0}

// Status is what the UI shows about the wireless server.
type Status struct {
	Running   bool     `json:"running"`
	Port      int      `json:"port"`
	Addresses []string `json:"addresses"`
	Device    *Device  `json:"device,omitempty"`
	Discovery []string `json:"discovery,omitempty"`
	LastError string   `json:"last_error,omitempty"`
}

// Device describes the reader currently connected.
type Device struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	App        string   `json:"app"`
	Address    string   `json:"address"`
	Extensions []string `json:"extensions"`
	FreeBytes  int64    `json:"free_bytes"`
}

type Server struct {
	mu        sync.Mutex
	listener  net.Listener
	udp       *net.UDPConn
	announce  *exec.Cmd
	port      int
	sess      *session
	lastError string
	opdsPort  func() int
	libUUID   string
	cancel    context.CancelFunc
}

// New returns a stopped server. opdsPort reports the OPDS port advertised in discovery
// replies (calibre advertises its content server port there).
func New(libraryUUID string, opdsPort func() int) *Server {
	return &Server{libUUID: libraryUUID, opdsPort: opdsPort}
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return nil
	}
	ln, port, err := listenTCP()
	if err != nil {
		return fmt.Errorf("não foi possível abrir a porta da conexão sem fio: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.listener, s.port, s.cancel, s.lastError = ln, port, cancel, ""

	if udp, err := listenBroadcast(); err == nil {
		s.udp = udp
		go s.answerDiscovery(udp)
	} else {
		log.Printf("wireless: discovery port unavailable: %v", err)
	}
	s.announce = startAnnouncement(ctx, port)
	go s.acceptLoop(ctx, ln)
	log.Printf("wireless: listening on port %d", port)
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	ln, udp, cancel, sess, ann := s.listener, s.udp, s.cancel, s.sess, s.announce
	s.listener, s.udp, s.cancel, s.sess, s.announce = nil, nil, nil, nil, nil
	s.mu.Unlock()
	if sess != nil {
		sess.eject()
	}
	if cancel != nil {
		cancel()
	}
	if ann != nil && ann.Process != nil {
		ann.Process.Kill()
	}
	if ln != nil {
		ln.Close()
	}
	if udp != nil {
		udp.Close()
	}
}

func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Running: s.listener != nil, Port: s.port, LastError: s.lastError}
	if st.Running {
		st.Addresses = LocalIPs()
		st.Discovery = []string{"broadcast"}
		if s.udp == nil {
			st.Discovery = nil
		}
		if s.announce != nil {
			st.Discovery = append(st.Discovery, "bonjour")
		}
	}
	if s.sess != nil {
		d := s.sess.device
		st.Device = &d
	}
	return st
}

// Session returns the connected device's session, or nil.
func (s *Server) Session() *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess
}

func listenTCP() (net.Listener, int, error) {
	try := []int{defaultPort}
	for i := 0; i < 20; i++ {
		try = append(try, 8192+rand.IntN(65525-8192))
	}
	var lastErr error
	for _, p := range try {
		ln, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", p))
		if err == nil {
			return ln, p, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

func listenBroadcast() (*net.UDPConn, error) {
	var lastErr error
	for _, p := range broadcastPorts {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: p})
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// answerDiscovery replies to any datagram on a broadcast port, as calibre does, with
// "calibre wireless device client (on HOST);CONTENT_PORT,DEVICE_PORT".
func (s *Server) answerDiscovery(udp *net.UDPConn) {
	host, _ := os.Hostname()
	host, _, _ = strings.Cut(host, ".")
	buf := make([]byte, 128)
	for {
		_, from, err := udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		s.mu.Lock()
		port := s.port
		s.mu.Unlock()
		reply := fmt.Sprintf("%s (on %s);%d,%d", clientString, host, s.opdsPort(), port)
		udp.WriteToUDP([]byte(reply), from)
	}
}

// startAnnouncement publishes the service over Bonjour/mDNS like calibre does, using
// the OS daemon (dns-sd on macOS, avahi on Linux). Returns nil when unavailable.
func startAnnouncement(ctx context.Context, port int) *exec.Cmd {
	var cmd *exec.Cmd
	name := "Lord Books"
	if p, err := exec.LookPath("dns-sd"); err == nil {
		cmd = exec.CommandContext(ctx, p, "-R", name, "_calibresmartdeviceapp._tcp", "local", fmt.Sprint(port))
	} else if p, err := exec.LookPath("avahi-publish-service"); err == nil {
		cmd = exec.CommandContext(ctx, p, name, "_calibresmartdeviceapp._tcp", fmt.Sprint(port))
	} else {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}
	go cmd.Wait()
	return cmd
}

func (s *Server) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handle(ctx, conn)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	s.mu.Lock()
	busy := s.sess
	s.mu.Unlock()
	if busy != nil {
		writeMessage(conn, opCalibreBusy, map[string]any{"otherDevice": busy.device.Name})
		conn.Close()
		return
	}

	sess := &session{conn: conn, r: bufio.NewReader(conn), libUUID: s.libUUID}
	if err := sess.handshake(); err != nil {
		log.Printf("wireless: handshake with %s failed: %v", conn.RemoteAddr(), err)
		s.mu.Lock()
		s.lastError = "Falha ao conectar com o aparelho: " + err.Error()
		s.mu.Unlock()
		conn.Close()
		return
	}
	s.mu.Lock()
	if s.listener == nil || s.sess != nil { // stopped or raced with another device
		s.mu.Unlock()
		sess.eject()
		return
	}
	s.sess, s.lastError = sess, ""
	s.mu.Unlock()
	log.Printf("wireless: %s connected from %s", sess.device.Name, conn.RemoteAddr())

	sess.keepAlive(ctx)

	s.mu.Lock()
	if s.sess == sess {
		s.sess = nil
	}
	s.mu.Unlock()
	log.Printf("wireless: %s disconnected", sess.device.Name)
}

// LocalIPs lists this machine's IPv4 addresses on active non-loopback interfaces,
// private-network addresses first, for the user to type on a reader if needed.
func LocalIPs() []string {
	var private, other []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			if ipn.IP.IsPrivate() {
				private = append(private, ipn.IP.String())
			} else {
				other = append(other, ipn.IP.String())
			}
		}
	}
	return append(private, other...)
}

var errNotConnected = errors.New("nenhum aparelho conectado por Wi-Fi")
