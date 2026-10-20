package pages

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func (s *Service) Moderate(ctx context.Context, r access.Request, id string, hidden bool, reason string, version int64) (Page, error) {
	id = strings.ToLower(id)
	if !identity.ValidID(id) {
		return Page{}, fault.NotFound
	}
	if !utf8.ValidString(reason) || strings.ContainsRune(reason, 0) || strings.TrimSpace(reason) == "" || utf8.RuneCountInString(reason) > 2000 || version < 0 {
		return Page{}, fault.Invalid
	}
	if len(r.Key) < 8 || len(r.Key) > 128 || r.Hash == "" || r.ID == "" {
		return Page{}, fault.BadRequest
	}
	r.Credentials.Write = true
	reason = s.text.NFC(strings.TrimSpace(reason))
	if utf8.RuneCountInString(reason) > 2000 {
		return Page{}, fault.Invalid
	}
	var out Page
	err := s.repo.WithinPages(ctx, func(tx Tx) error {
		if _, err := identity.Require(ctx, tx, r.Credentials); err != nil {
			return err
		}
		actor, policy, err := access.Authorize(ctx, tx, r.Credentials, id, "manage", true)
		if err != nil {
			return err
		}
		p, err := tx.Page(ctx, id)
		if err != nil {
			return err
		}
		operation := "page.moderate:" + id
		raw, err := tx.Replay(ctx, actor.ID, operation, r.Key, r.Hash)
		if err != nil {
			return err
		}
		if raw != nil {
			return json.Unmarshal(raw, &out)
		}
		if version != policy.Version {
			return fault.Stale
		}
		if hidden == (policy.Status == "hidden") {
			return fault.State
		}
		policy.Status = "hidden"
		if !hidden {
			policy.Status = "proposed"
			if p.HeadRevisionID != nil {
				policy.Status = "published"
			}
		}
		policy.Version++
		if err := tx.SavePolicy(ctx, policy); err != nil {
			return err
		}
		if err := tx.Moderation(ctx, access.Moderation{Actor: actor, PageID: id, TargetID: id, Kind: "page", Hidden: hidden, Reason: reason, Version: policy.Version, RequestID: r.ID}); err != nil {
			return err
		}
		if err := tx.Audit(ctx, actor, "page.moderate", id, r.ID); err != nil {
			return err
		}
		p.Status = policy.Status
		p.PolicyVersion = policy.Version
		out = p
		raw, err = json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Remember(ctx, actor.ID, operation, r.Key, r.Hash, raw)
	})
	if err != nil {
		return Page{}, err
	}
	return out, nil
}
