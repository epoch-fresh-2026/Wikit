package pages

import (
	"context"
	"encoding/json"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

// directory runs inside the caller's current authorization transaction.
func (s *Service) directory(ctx context.Context, tx Tx, actor identity.Actor, groups []access.Group, q Query, binding string) (List, error) {
	out := List{Items: []json.RawMessage{}}
	anchor := "0"
	if q.Cursor != "" {
		var err error
		anchor, err = tx.PageAnchor(ctx, q.Cursor, actor.ID, binding, q.Limit)
		if err != nil {
			return out, err
		}
	}
	selected := []string{}
	policies := map[string]access.Policy{}
	more := false
	scanned := 0
	for scanned < 1000 && len(selected) < q.Limit {
		size := min(100, 1000-scanned)
		batch, err := tx.PageBatch(ctx, q.Filter, q.Type, anchor, size)
		if err != nil {
			return out, err
		}
		ids := make([]string, 0, len(batch))
		for _, p := range batch {
			ids = append(ids, p.ID)
		}
		current, err := tx.Policies(ctx, ids)
		if err != nil {
			return out, err
		}
		batchPolicies := map[string]access.Policy{}
		for _, p := range current {
			batchPolicies[p.PageID] = p
		}
		more = len(batch) == size
		for i, p := range batch {
			anchor = p.ID
			scanned++
			policy, ok := batchPolicies[p.ID]
			if !ok {
				return out, fault.NotFound
			}
			if p.Status != "published" || p.HeadRevisionID == nil || access.CheckPolicy(actor, policy, groups, "read", false) != nil {
				continue
			}
			selected = append(selected, p.ID)
			policies[p.ID] = policy
			if len(selected) == q.Limit {
				more = more || i+1 < len(batch)
				break
			}
		}
		if !more {
			break
		}
	}
	details, err := tx.PageDetails(ctx, selected)
	if err != nil {
		return out, err
	}
	byID := map[string]Detail{}
	for _, d := range details {
		byID[d.Page.ID] = d
	}
	for _, id := range selected {
		d, ok := byID[id]
		if !ok {
			return out, fault.NotFound
		}
		d.Capabilities = access.Capabilities(actor, policies[id], groups)
		raw, err := json.Marshal(d)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, raw)
	}
	if more {
		plain, err := s.secrets.Secret()
		if err != nil {
			return out, err
		}
		next, err := tx.NewPageCursor(ctx, plain, actor.ID, binding, anchor, q.Limit)
		if err != nil {
			return out, err
		}
		out.Next = &next
	}
	return out, nil
}
