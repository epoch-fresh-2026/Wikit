//go:build production

package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var bundle embed.FS

func Handler() (http.Handler, error) {
	files, err := fs.Sub(bundle, "dist")
	if err != nil {
		return nil, err
	}
	return New(files)
}
