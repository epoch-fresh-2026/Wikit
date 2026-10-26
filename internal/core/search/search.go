package search

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type Query struct {
	Q, Tag, Cursor, Type string
	TagSet               bool
	Filter               content.TagFilter
	Limit                int
}
type Hit struct {
	PageID     string    `json:"page_id"`
	RevisionID string    `json:"revision_id"`
	Type       string    `json:"type"`
	Title      string    `json:"title"`
	Tags       []string  `json:"tags"`
	Snippet    string    `json:"snippet"`
	Passages   []Passage `json:"passages"`
}
type Result struct {
	Items []Hit   `json:"items"`
	Next  *string `json:"next_cursor"`
}

func valid(s string, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) <= max
}
func (s *Service) Search(ctx context.Context, c identity.Credentials, q Query) (Result, error) {
	if !valid(q.Q, 500) || strings.TrimSpace(q.Q) == "" || (q.Type != "" && q.Type != "document" && q.Type != "media") || q.Limit < 1 || q.Limit > 100 {
		return Result{}, fault.BadRequest
	}
	q.Q = s.text.NFC(strings.TrimSpace(q.Q))
	var err error
	q.Filter, err = q.Filter.Normalize(s.text, q.Tag, q.TagSet)
	if err != nil {
		return Result{}, err
	}
	if !valid(q.Q, 500) {
		return Result{}, fault.BadRequest
	}
	var index, binding, actorID string
	var filter Filter
	offset := 0
	c.Write = false
	err = s.repo.WithinSearch(ctx, func(tx Tx) error {
		a, err := identity.Resolve(ctx, tx, c)
		if err != nil {
			return err
		}
		actorID = a.ID
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		filter = Filter{Actor: a.ID, Admin: a.Role == "admin", Tags: q.Filter, Type: q.Type, Groups: []string{}}
		for _, g := range groups {
			if a.ID != "" && slices.Contains(g.Members, a.ID) {
				filter.Groups = append(filter.Groups, g.ID)
			}
		}
		index, err = tx.ActiveIndex(ctx)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(struct {
			Q, Type, Index string
			Tags           content.TagFilter
		}{q.Q, q.Type, index, q.Filter})
		if err != nil {
			return err
		}
		binding = "search:" + identity.Digest(string(raw))
		if q.Cursor != "" {
			offset, err = tx.Cursor(ctx, q.Cursor, a.ID, binding, q.Limit)
		}
		return err
	})
	if err != nil {
		return Result{}, err
	}

	// Network requests remain outside the authorization transaction. Carry only
	// selected candidates between batches, never their authority or content.
	type positioned struct {
		Candidate
		Position int
	}
	selected := []positioned{}
	position, fetched := offset, 0
	for fetched < 1000 {
		n := min(100, max(q.Limit, fetched), 1000-fetched)
		batch, err := s.index.Search(ctx, index, q.Q, filter, position, n)
		if err != nil {
			return Result{}, fault.SearchUnavailable
		}
		if len(batch.Hits) > n {
			return Result{}, fault.SearchUnavailable
		}
		candidates := append([]positioned{}, selected...)
		for _, candidate := range batch.Hits {
			position++
			candidates = append(candidates, positioned{candidate, position})
		}
		fetched += len(batch.Hits)
		more := batch.More && len(batch.Hits) > 0
		out := Result{Items: []Hit{}}
		selected = nil
		err = s.repo.WithinSearch(ctx, func(tx Tx) error {
			a, err := identity.Resolve(ctx, tx, c)
			if err != nil {
				return err
			}
			if a.ID != actorID {
				return fault.BadRequest
			}
			current, err := tx.ActiveIndex(ctx)
			if err != nil {
				return err
			}
			if current != index {
				return fault.BadRequest
			}
			groups, err := tx.Groups(ctx)
			if err != nil {
				return err
			}
			ids := []string{}
			unique := map[string]bool{}
			for _, candidate := range candidates {
				if access.ValidPageID(candidate.ID) && identity.ValidID(candidate.Revision) && !unique[candidate.ID] {
					ids = append(ids, candidate.ID)
					unique[candidate.ID] = true
				}
			}
			sources, err := tx.SearchSources(ctx, ids)
			if err != nil {
				return err
			}
			byID := map[string]Source{}
			for _, source := range sources {
				byID[source.Page.ID] = source
			}
			consumed := position
			seen := map[string]bool{}
			repairs := map[string]bool{}
			repair := func(id string) error {
				if repairs[id] {
					return nil
				}
				repairs[id] = true
				return tx.Repair(ctx, id)
			}
			for _, candidate := range candidates {
				if !access.ValidPageID(candidate.ID) || !identity.ValidID(candidate.Revision) {
					continue
				}
				source, ok := byID[candidate.ID]
				if !ok {
					continue
				}
				if source.Page.HeadRevisionID == nil || source.Page.Status != "published" {
					if err := repair(candidate.ID); err != nil {
						return err
					}
					continue
				}
				if access.CheckPolicy(a, source.Policy, groups, "read", false) != nil {
					continue
				}
				if *source.Page.HeadRevisionID != candidate.Revision || source.Policy.Version != candidate.PolicyVersion {
					if err := repair(candidate.ID); err != nil {
						return err
					}
					continue
				}
				if !q.Filter.Matches(source.Snapshot.Tags) || (q.Type != "" && q.Type != source.Page.Type) {
					continue
				}
				if seen[candidate.ID] {
					continue
				}
				seen[candidate.ID] = true
				selected = append(selected, candidate)
				if len(selected) == q.Limit {
					consumed = candidate.Position
					break
				}
			}
			// Incomplete batches only carry candidate identifiers to the next iteration.
			// This transaction is the final authority whenever a response is returned.
			if len(selected) < q.Limit && more && fetched < 1000 {
				return nil
			}
			for _, candidate := range selected {
				source := byID[candidate.ID]
				evidence, err := passages(source, q.Q, s.text)
				if err != nil {
					return err
				}
				parts := []string{}
				for _, passage := range evidence {
					parts = append(parts, passage.Text)
				}
				out.Items = append(out.Items, Hit{source.Page.ID, candidate.Revision, source.Page.Type, source.Snapshot.Title, source.Snapshot.Tags, bounded(strings.Join(parts, "\n\n"), 2000), evidence})
			}
			if consumed < position || more {
				plain, err := s.secrets.Secret()
				if err != nil {
					return err
				}
				next, err := tx.NewCursor(ctx, plain, a.ID, binding, consumed, q.Limit)
				if err != nil {
					return err
				}
				out.Next = &next
			}
			return nil
		})
		if err != nil {
			return Result{}, err
		}
		if len(selected) == q.Limit || !more || fetched >= 1000 {
			return out, nil
		}
	}
	return Result{}, fault.SearchUnavailable
}

func project(s Source, text Text) (*Document, error) {
	if s.Page.HeadRevisionID == nil || s.Page.Status != "published" {
		return nil, nil
	}
	markdown := ""
	if s.Snapshot.Markdown != nil {
		markdown = *s.Snapshot.Markdown
	} else if s.Snapshot.Description != nil {
		markdown = *s.Snapshot.Description
	}
	body, err := text.Plain(markdown)
	if err != nil {
		return nil, err
	}
	d := &Document{ID: s.Page.ID, Revision: *s.Page.HeadRevisionID, PolicyVersion: s.Policy.Version, Type: s.Page.Type, Title: s.Snapshot.Title, Tags: s.Snapshot.Tags, Body: body, Visibility: s.Policy.Visibility, Users: []string{}, Groups: []string{}}
	for _, grant := range s.Policy.Grants {
		if !slices.Contains(grant.Capabilities, "read") {
			continue
		}
		if grant.SubjectType == "user" {
			d.Users = append(d.Users, grant.SubjectID)
		} else if grant.SubjectType == "group" {
			d.Groups = append(d.Groups, grant.SubjectID)
		}
	}
	return d, nil
}
