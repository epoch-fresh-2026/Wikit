package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func (a API) discussionRoute(r *http.Request, c identity.Credentials, requestID, path, ip string) (any, int, error, bool) {
	p := strings.Split(path, "/")
	if len(p) < 2 || len(p) > 3 {
		return nil, 0, nil, false
	}
	id := p[1]
	action := ""
	if len(p) == 3 {
		action = p[2]
	}
	recognized := p[0] == "pages" && (action == "issues" || action == "moderation") || p[0] == "issues" && (action == "" || action == "comments" || action == "state" || action == "proposals" || action == "moderation") || p[0] == "comments" && action == "moderation"
	if !recognized {
		return nil, 0, nil, false
	}
	if r.Method == "GET" {
		hidden := false
		switch r.URL.Query().Get("include_hidden") {
		case "", "false":
		case "true":
			hidden = true
		default:
			return nil, 0, fault.BadRequest, true
		}
		if p[0] == "issues" && action == "" {
			out, err := a.Discussions.Get(r.Context(), c, id, hidden)
			return out, 200, err, true
		}
		if action == "issues" || action == "comments" {
			q := discussions.Query{Kind: action, ID: id, IncludeHidden: hidden, Limit: 20, Cursor: r.URL.Query().Get("cursor")}
			if action == "issues" {
				q.State = r.URL.Query().Get("state")
			}
			if raw := r.URL.Query().Get("limit"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil {
					return nil, 0, fault.BadRequest, true
				}
				q.Limit = n
			}
			out, err := a.Discussions.List(r.Context(), c, q)
			return out, 200, err, true
		}
	}
	if r.Method != "POST" {
		return nil, 0, fault.NotFound, true
	}
	switch action {
	case "issues":
		var in struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		raw, err := decodeLimit(r, &in, 1<<20, "title", "body")
		if err != nil {
			return nil, 0, err, true
		}
		out, err := a.Discussions.Create(r.Context(), request(r, c, requestID, raw), id, in.Title, in.Body, ip)
		return out, 201, err, true
	case "comments":
		var in struct {
			Body string `json:"body"`
		}
		raw, err := decodeLimit(r, &in, 1<<20, "body")
		if err != nil {
			return nil, 0, err, true
		}
		out, err := a.Discussions.Comment(r.Context(), request(r, c, requestID, raw), id, in.Body, ip)
		return out, 201, err, true
	case "state":
		var in struct {
			State   string `json:"state"`
			Version int64  `json:"expected_version"`
		}
		raw, err := decode(r, &in, "state", "expected_version")
		if err != nil {
			return nil, 0, err, true
		}
		out, err := a.Discussions.State(r.Context(), request(r, c, requestID, raw), id, in.State, in.Version)
		return out, 200, err, true
	case "proposals":
		var in struct {
			Proposal string `json:"proposal_id"`
			Version  int64  `json:"expected_version"`
		}
		raw, err := decode(r, &in, "proposal_id", "expected_version")
		if err != nil {
			return nil, 0, err, true
		}
		out, err := a.Discussions.Link(r.Context(), request(r, c, requestID, raw), id, in.Proposal, in.Version)
		return out, 200, err, true
	case "moderation":
		var in struct {
			Hidden  bool   `json:"hidden"`
			Reason  string `json:"reason"`
			Version int64  `json:"expected_version"`
		}
		raw, err := decode(r, &in, "hidden", "reason", "expected_version")
		if err != nil {
			return nil, 0, err, true
		}
		req := request(r, c, requestID, raw)
		if p[0] == "pages" {
			out, err := a.Pages.Moderate(r.Context(), req, id, in.Hidden, in.Reason, in.Version)
			return out, 200, err, true
		}
		scope := "issue"
		if p[0] == "comments" {
			scope = "comment"
		}
		out, err := a.Discussions.Moderate(r.Context(), req, scope, id, in.Hidden, in.Reason, in.Version)
		return out, 200, err, true
	}
	return nil, 0, fault.NotFound, true
}
