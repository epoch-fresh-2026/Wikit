package discussions

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func (s *Service) Get(ctx context.Context, c identity.Credentials, id string, includeHidden bool) (Issue, error) {
	var out Issue
	err := s.repo.WithinDiscussions(ctx, func(tx Tx) error {
		v, err := load(ctx, tx, c, "issue", id, includeHidden)
		if err == nil {
			out = v.issue
		}
		return err
	})
	if err != nil {
		return Issue{}, err
	}
	return out, nil
}

type Query struct {
	Kind, ID, State, Cursor string
	IncludeHidden           bool
	Limit                   int
}
type List struct {
	Items []json.RawMessage `json:"items"`
	Next  *string           `json:"next_cursor"`
}

func (s *Service) List(ctx context.Context, c identity.Credentials, q Query) (List, error) {
	q.ID = strings.ToLower(q.ID)
	if q.Limit < 1 || q.Limit > 100 || q.Kind != "issues" && q.Kind != "comments" || q.State != "" && (q.Kind == "comments" || q.State != "open" && q.State != "closed") {
		return List{}, fault.BadRequest
	}
	var out List
	err := s.repo.WithinDiscussions(ctx, func(tx Tx) error {
		scope := "page"
		if q.Kind == "comments" {
			scope = "issue"
		}
		v, err := load(ctx, tx, c, scope, q.ID, q.IncludeHidden)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(struct {
			Kind, ID, State string
			Hidden          bool
		}{q.Kind, q.ID, q.State, q.IncludeHidden})
		if err != nil {
			return err
		}
		binding := "discussions:" + identity.Digest(string(encoded))
		offset := 0
		if q.Cursor != "" {
			offset, err = tx.Cursor(ctx, q.Cursor, v.actor.ID, binding, q.Limit)
			if err != nil {
				return err
			}
		}
		var rows []Summary
		if q.Kind == "issues" {
			rows, err = tx.IssueSummaries(ctx, q.ID, q.State)
		} else {
			rows, err = tx.CommentSummaries(ctx, q.ID)
		}
		if err != nil {
			return err
		}
		ids := []string{}
		for _, row := range rows {
			if !row.Hidden || q.IncludeHidden {
				ids = append(ids, row.ID)
			}
		}
		if offset > len(ids) {
			return fault.BadRequest
		}
		end := min(offset+q.Limit, len(ids))
		out.Items = []json.RawMessage{}
		for _, id := range ids[offset:end] {
			var value any
			if q.Kind == "issues" {
				value, err = tx.Issue(ctx, id)
			} else {
				value, err = tx.Comment(ctx, id)
			}
			if err != nil {
				return err
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, raw)
		}
		if end < len(ids) {
			plain, err := s.secrets.Secret()
			if err != nil {
				return err
			}
			next, err := tx.NewCursor(ctx, plain, v.actor.ID, binding, end, q.Limit)
			if err != nil {
				return err
			}
			out.Next = &next
		}
		return nil
	})
	if err != nil {
		return List{}, err
	}
	return out, nil
}
