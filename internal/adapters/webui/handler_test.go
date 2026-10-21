package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticRoutesAndProtocolIsolation(t *testing.T) {
	h, err := New(fstest.MapFS{"index.html": {Data: []byte("<html>Wikit</html>")}, "assets/app.js": {Data: []byte("console.log('ok')")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/pages/some-id", 200, "<html>Wikit</html>"},
		{"GET", "/assets/app.js", 200, "console.log('ok')"},
		{"HEAD", "/pages/one", 200, ""},
		{"GET", "/assets/missing.js", 404, "404 page not found\n"},
		{"GET", "/assets/", 404, "404 page not found\n"},
		{"GET", "/api/v1/missing", 404, "404 page not found\n"},
		{"GET", "/mcp", 404, "404 page not found\n"},
		{"GET", "/health/missing", 404, "404 page not found\n"},
		{"POST", "/login", 405, "method not allowed\n"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != tt.status || w.Body.String() != tt.body {
			t.Errorf("%s %s: %d %q", tt.method, tt.path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("missing browser protections")
		}
	}
	if _, err := New(fstest.MapFS{}); err == nil {
		t.Fatal("missing bundle accepted")
	}
}
