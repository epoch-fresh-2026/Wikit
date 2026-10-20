package http

import (
	"net/http"
	"strconv"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

func (a API) searchRoute(r *http.Request, c identity.Credentials) (any, int, error) {
	for key, values := range r.URL.Query() {
		if len(values) != 1 {
			return nil, 0, fault.BadRequest
		}
		switch key {
		case "q", "tag", "limit", "cursor":
		default:
			return nil, 0, fault.BadRequest
		}
	}
	q := search.Query{Q: r.URL.Query().Get("q"), Tag: r.URL.Query().Get("tag"), Cursor: r.URL.Query().Get("cursor"), Limit: 20}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, 0, fault.BadRequest
		}
		q.Limit = n
	}
	out, err := a.Search.Search(r.Context(), c, q)
	return out, 200, err
}
