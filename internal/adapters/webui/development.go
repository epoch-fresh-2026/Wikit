//go:build !production

package webui

import "net/http"

func Handler() (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "WebUI not embedded; use the Vite development server", http.StatusServiceUnavailable)
	}), nil
}
