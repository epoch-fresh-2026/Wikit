package http

import (
	"net/http"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
)

// Only the head is nullable; its presence remains mandatory.
func nullableDecode(r *http.Request, dst any, fields ...string) ([]byte, error) {
	return decodeFields(r, dst, 5<<20, map[string]bool{"expected_head_revision_id": true}, fields...)
}
func (a API) proposalRoute(r *http.Request, c identity.Credentials, requestID, path string) (any, int, error, bool) {
	p := strings.Split(path, "/")
	if len(p) != 3 {
		return nil, 0, nil, false
	}
	id, action := p[1], p[2]
	if p[0] == "proposals" && r.Method == "GET" {
		switch action {
		case "diff":
			out, err := a.Proposals.Diff(r.Context(), c, id)
			return out, 200, err, true

		}
		return nil, 0, nil, false
	}
	if r.Method != "POST" {
		return nil, 0, nil, false
	}
	if p[0] == "pages" {
		switch action {
		case "proposals":
			var in struct {
				Head     *string          `json:"expected_head_revision_id"`
				Snapshot content.Snapshot `json:"snapshot"`
			}
			body, err := nullableDecode(r, &in, "expected_head_revision_id", "snapshot")
			if err != nil {
				return nil, 0, err, true
			}
			out, err := a.Proposals.Create(r.Context(), request(r, c, requestID, body), id, in.Head, in.Snapshot)
			return out, 201, err, true
		case "revert", "restore":
			var in struct {
				Revision string `json:"revision_id"`
				Head     string `json:"expected_head_revision_id"`
			}
			body, err := decode(r, &in, "revision_id", "expected_head_revision_id")
			if err != nil {
				return nil, 0, err, true
			}
			out, err := a.Proposals.Rollback(r.Context(), request(r, c, requestID, body), id, action, in.Revision, in.Head)
			return out, 201, err, true
		}
		return nil, 0, nil, false
	}
	if p[0] != "proposals" {
		return nil, 0, nil, false
	}
	switch action {
	case "revisions":
		var in struct {
			proposals.Expected
			Snapshot content.Snapshot `json:"snapshot"`
		}
		body, err := nullableDecode(r, &in, "expected_head_revision_id", "expected_proposal_revision_id", "snapshot")
		if err != nil {
			return nil, 0, err, true
		}
		out, err := a.Proposals.Update(r.Context(), request(r, c, requestID, body), id, in.Expected, in.Snapshot)
		return out, 200, err, true
	case "rebase", "merge":
		var in proposals.Expected
		body, err := nullableDecode(r, &in, "expected_head_revision_id", "expected_proposal_revision_id")
		if err != nil {
			return nil, 0, err, true
		}
		req := request(r, c, requestID, body)
		if action == "rebase" {
			out, err := a.Proposals.Rebase(r.Context(), req, id, in)
			return out, 200, err, true
		}
		out, err := a.Proposals.Merge(r.Context(), req, id, in)
		return out, 200, err, true
	case "state":
		var in struct {
			State   string `json:"state"`
			Version int64  `json:"expected_proposal_version"`
		}
		body, err := decode(r, &in, "state", "expected_proposal_version")
		if err != nil {
			return nil, 0, err, true
		}
		out, err := a.Proposals.State(r.Context(), request(r, c, requestID, body), id, in.State, in.Version)
		return out, 200, err, true
	}
	return nil, 0, nil, false
}
