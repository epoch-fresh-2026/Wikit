package http

import (
	"net/http"
	"strconv"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

func (a API) searchRoute(r *http.Request, c identity.Credentials) (any, int, error) {
	values := r.URL.Query()
	filter, err := tagQuery(values, true)
	if err != nil {
		return nil, 0, err
	}
	q := search.Query{Q: values.Get("q"), Tag: values.Get("tag"), TagSet: values.Has("tag"), Filter: filter, Type: values.Get("type"), Cursor: values.Get("cursor"), Limit: 20}
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
