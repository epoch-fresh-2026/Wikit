package http

import (
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func (a API) mediaRoute(w http.ResponseWriter, r *http.Request, c identity.Credentials, id, ip string) bool {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(path, "/")
	respond := func(out any, status int, err error) bool {
		if err != nil {
			failure(w, err, id)
		} else {
			response(w, status, out)
		}
		return true
	}
	if path == "uploads" && r.Method == "POST" {
		var in struct {
			Filename string `json:"filename"`
			Mime     string `json:"mime"`
			Size     int64  `json:"size"`
		}
		body, err := decode(r, &in, "filename", "size", "mime")
		if err != nil {
			return respond(nil, 0, err)
		}
		out, err := a.Media.Prepare(r.Context(), request(r, c, id, body), in.Filename, in.Mime, in.Size, ip)
		return respond(out, 201, err)
	}
	if len(parts) == 2 && parts[0] == "uploads" && r.Method == "GET" {
		out, err := a.Media.Get(r.Context(), c, parts[1])
		return respond(out, 200, err)
	}
	if len(parts) == 3 && parts[0] == "uploads" && parts[2] == "content" && r.Method == "PUT" {
		controller := http.NewResponseController(w)
		for _, set := range []func(time.Time) error{controller.SetReadDeadline, controller.SetWriteDeadline} {
			if err := set(time.Now().Add(15 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return respond(nil, 0, fault.StorageUnavailable)
			}
		}
		mimeType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			return respond(nil, 0, fault.UnsupportedMedia)
		}
		out, err := a.Media.Put(r.Context(), c, parts[1], mimeType, id, ip, r.Body)
		return respond(out, 200, err)
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		return false
	}
	kind, target, revision := "", "", ""
	if len(parts) == 5 && parts[0] == "pages" && parts[2] == "revisions" && parts[4] == "content" {
		kind, target, revision = "published", parts[1], parts[3]
	}
	if len(parts) == 5 && parts[0] == "proposals" && parts[2] == "revisions" && parts[4] == "content" {
		kind, target, revision = "proposal", parts[1], parts[3]
	}
	if kind == "" {
		return false
	}
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Now().Add(15 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return respond(nil, 0, fault.StorageUnavailable)
	}
	download, err := a.Media.Read(r.Context(), c, kind, target, revision)
	if err != nil {
		return respond(nil, 0, err)
	}
	defer func() {
		if err := download.Reader.Close(); err != nil {
			return
		}
	}()
	w.Header().Set("Content-Type", download.Blob.Mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("ETag", `"`+download.Blob.Digest+`"`)
	disposition := "inline"
	if download.Blob.Mime == "application/pdf" || r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": download.Blob.ID + extension(download.Blob.Mime)}))
	// ServeContent removes cache headers on errors (e.g. 416); restore the
	// private policy at WriteHeader, after all conditional/range handling.
	http.ServeContent(privateWriter{w}, r, "", download.CreatedAt, download.Reader)
	return true
}

type privateWriter struct{ http.ResponseWriter }

func (w privateWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w privateWriter) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Cookie, Authorization")
	w.ResponseWriter.WriteHeader(code)
}
func (w privateWriter) Write(b []byte) (int, error) { return w.ResponseWriter.Write(b) }
func extension(mime string) string {
	return map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp", "audio/mpeg": ".mp3", "audio/wav": ".wav", "audio/ogg": ".ogg", "video/mp4": ".mp4", "video/webm": ".webm", "application/pdf": ".pdf"}[mime]
}
