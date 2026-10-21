// Package webui serves the compiled application without handling business data.
package webui

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

func New(files fs.FS) (http.Handler, error) {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil || len(index) == 0 {
		return nil, errors.New("WebUI bundle is missing")
	}
	assets := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; media-src 'self' blob:; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		for _, reserved := range []string{"api", "health", "mcp"} {
			if name == reserved || strings.HasPrefix(name, reserved+"/") {
				http.NotFound(w, r)
				return
			}
		}
		if strings.HasPrefix(name, "assets/") {
			info, err := fs.Stat(files, name)
			if err != nil || info.IsDir() {
				http.NotFound(w, r)
				return
			}
			assets.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	}), nil
}
