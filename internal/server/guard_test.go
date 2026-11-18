package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := guard(ok)

	cases := []struct {
		name    string
		method  string
		remote  string
		host    string
		headers map[string]string
		want    int
	}{
		{"local read", "GET", "127.0.0.1:5000", "127.0.0.1:8484", nil, 200},
		{"localhost name", "GET", "127.0.0.1:5000", "localhost:8484", nil, 200},
		{"ipv6 loopback", "GET", "[::1]:5000", "[::1]:8484", nil, 200},
		{"same-origin write", "POST", "127.0.0.1:5000", "127.0.0.1:8484",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:8484"}, 200},
		{"tool without browser headers", "DELETE", "127.0.0.1:5000", "127.0.0.1:8484", nil, 200},

		{"another computer on the network", "GET", "192.168.0.50:5000", "192.168.0.10:8484", nil, 403},
		{"another computer, write", "DELETE", "192.168.0.50:5000", "192.168.0.10:8484", nil, 403},
		{"DNS rebinding", "GET", "127.0.0.1:5000", "evil.example:8484", nil, 403},
		{"cross-site form post", "POST", "127.0.0.1:5000", "127.0.0.1:8484",
			map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"same-site but other origin", "POST", "127.0.0.1:5000", "127.0.0.1:8484",
			map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"old browser, foreign Origin", "PUT", "127.0.0.1:5000", "127.0.0.1:8484",
			map[string]string{"Origin": "https://evil.example"}, 403},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "/api/books/1", nil)
		req.RemoteAddr = c.remote
		req.Host = c.host
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}
}
