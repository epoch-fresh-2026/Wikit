package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
)

func (a API) pageRoute(r *http.Request, c identity.Credentials, requestID, path, ip string) (any, int, error) {
	if path == "pages" && r.Method == "POST" {
		var input struct {
			Visibility string           `json:"visibility"`
			Snapshot   content.Snapshot `json:"snapshot"`
		}
		body, err := decodeLimit(r, &input, 5<<20, "visibility", "snapshot")
		if err != nil {
			return nil, 0, err
		}
		out, err := a.Pages.Create(r.Context(), request(r, c, requestID, body), input.Visibility, input.Snapshot, ip)
		return out, 201, err
	}
	if r.Method != "GET" {
		return nil, 0, fault.NotFound
	}
	parts := strings.Split(path, "/")
	q := pages.Query{Limit: 20, Cursor: r.URL.Query().Get("cursor"), Tag: r.URL.Query().Get("tag"), Type: r.URL.Query().Get("type"), State: r.URL.Query().Get("state")}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil {
			return nil, 0, fault.BadRequest
		}
		q.Limit = value
	}
	switch {
	case path == "pages":
		q.Kind = "pages"
	case path == "tags":
		q.Kind = "tags"
	case path == "proposals":
		q.Kind = "proposals"
	case len(parts) == 2 && parts[0] == "pages":
		out, err := a.Pages.Get(r.Context(), c, parts[1], "")
		return out, 200, err
	case len(parts) == 3 && parts[0] == "pages" && (parts[2] == "revisions" || parts[2] == "proposals"):
		q.Kind = parts[2]
		q.PageID = parts[1]
	case len(parts) == 4 && parts[0] == "pages" && parts[2] == "revisions":
		out, err := a.Pages.Get(r.Context(), c, parts[1], parts[3])
		return out, 200, err
	case len(parts) == 2 && parts[0] == "proposals":
		out, err := a.Pages.Proposal(r.Context(), c, parts[1])
		return out, 200, err
	case len(parts) == 3 && parts[0] == "proposals" && parts[2] == "revisions":
		q.Kind = "candidates"
		q.ProposalID = parts[1]
	default:
		return nil, 0, fault.NotFound
	}
	out, err := a.Pages.List(r.Context(), c, q)
	return out, 200, err
}
