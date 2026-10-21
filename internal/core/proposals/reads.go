package proposals

import (
	"context"
	"fmt"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func formatRange(bs, bc, ps, pc int) string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", bs, bc, ps, pc)
}
func snapshotAt(ctx context.Context, tx Tx, page string, id *string, kind string) (content.Snapshot, error) {
	if id == nil {
		return emptySnapshot(kind), nil
	}
	v, err := tx.Revision(ctx, page, *id)
	return v.Snapshot, err
}
func read(ctx context.Context, s *Service, c identity.Credentials, id string, fn func(Tx, Proposal, content.Page, identity.Actor) error) error {
	id = strings.ToLower(id)
	if !identity.ValidID(id) {
		return fault.NotFound
	}
	return s.repo.WithinProposals(ctx, func(tx Tx) error {
		p, err := tx.Proposal(ctx, id)
		if err != nil {
			return err
		}
		a, _, err := access.Authorize(ctx, tx, c, p.PageID, "read", false)
		if err != nil {
			return err
		}
		page, err := tx.Page(ctx, p.PageID)
		if err != nil {
			return err
		}
		return fn(tx, p, page, a)
	})
}
func (s *Service) Diff(ctx context.Context, c identity.Credentials, id string) (Diff, error) {
	var out Diff
	err := read(ctx, s, c, id, func(tx Tx, p Proposal, page content.Page, a identity.Actor) error {
		b, err := snapshotAt(ctx, tx, p.PageID, p.CurrentRevision.BaseRevisionID, page.Type)
		if err != nil {
			return err
		}
		before, err := fields(b)
		if err != nil {
			return err
		}
		after, err := fields(p.CurrentRevision.Snapshot)
		if err != nil {
			return err
		}
		out = Diff{BaseRevisionID: p.CurrentRevision.BaseRevisionID, HeadRevisionID: page.HeadRevisionID, ProposalRevisionID: p.CurrentRevision.ID, Stale: !same(p.CurrentRevision.BaseRevisionID, page.HeadRevisionID), Fields: []Field{}}
		for _, path := range []string{"/title", "/tags", "/markdown", "/description", "/blob_id"} {
			if same(before[path], after[path]) {
				continue
			}
			f := Field{Path: path, Before: before[path], After: after[path]}
			if path == "/markdown" {
				d, err := unified(ctx, s.lines, *f.Before, *f.After)
				if err != nil {
					return err
				}
				f.UnifiedDiff = &d
			}
			out.Fields = append(out.Fields, f)
		}
		return nil
	})
	return out, err
}

func (s *Service) Rebase(ctx context.Context, r access.Request, id string, e Expected) (Proposal, error) {
	return write(ctx, s, r, id, "proposal.rebase", false, func(tx Tx, v loaded) (Proposal, error) {
		if err := open(v.proposal); err != nil {
			return Proposal{}, err
		}
		if err := editor(ctx, tx, v); err != nil {
			return Proposal{}, err
		}
		if err := permission(ctx, tx, v, "propose"); err != nil {
			return Proposal{}, err
		}
		if err := e.check(v.proposal, v.page, false); err != nil {
			return Proposal{}, err
		}
		base, err := snapshotAt(ctx, tx, v.page.ID, v.proposal.CurrentRevision.BaseRevisionID, v.page.Type)
		if err != nil {
			return Proposal{}, err
		}
		head, err := snapshotAt(ctx, tx, v.page.ID, v.page.HeadRevisionID, v.page.Type)
		if err != nil {
			return Proposal{}, err
		}
		merged, err := mergeSnapshots(ctx, s.lines, base, head, v.proposal.CurrentRevision.Snapshot)
		if err != nil {
			return Proposal{}, err
		}
		snapshot, refs, err := s.prepare(ctx, tx, r.Credentials, v, merged)
		if err != nil {
			return Proposal{}, err
		}
		return s.candidate(ctx, tx, v, snapshot, refs)
	})
}
func (s *Service) Rollback(ctx context.Context, r access.Request, page, kind, revision, head string) (Proposal, error) {
	if kind != "revert" && kind != "restore" {
		return Proposal{}, fault.Invalid
	}
	return write(ctx, s, r, page, "page."+kind, true, func(tx Tx, v loaded) (Proposal, error) {
		if err := permission(ctx, tx, v, "propose"); err != nil {
			return Proposal{}, err
		}
		revision, head = strings.ToLower(revision), strings.ToLower(head)
		if !identity.ValidID(revision) || !identity.ValidID(head) {
			return Proposal{}, fault.Invalid
		}
		if v.page.HeadRevisionID == nil || head != *v.page.HeadRevisionID {
			return Proposal{}, fault.Stale
		}
		target, err := tx.Revision(ctx, v.page.ID, revision)
		if err != nil {
			return Proposal{}, err
		}
		// Validate the selected original even if an inverse merge drops a reference.
		snapshot, _, err := s.prepare(ctx, tx, r.Credentials, v, target.Snapshot)
		if err != nil {
			return Proposal{}, err
		}
		if kind == "revert" {
			if target.ParentID == nil {
				return Proposal{}, fault.State
			}
			parent, err := snapshotAt(ctx, tx, v.page.ID, target.ParentID, v.page.Type)
			if err != nil {
				return Proposal{}, err
			}
			current, err := snapshotAt(ctx, tx, v.page.ID, v.page.HeadRevisionID, v.page.Type)
			if err != nil {
				return Proposal{}, err
			}
			snapshot, err = mergeSnapshots(ctx, s.lines, target.Snapshot, current, parent)
			if err != nil {
				return Proposal{}, err
			}
		}
		snapshot, refs, err := s.prepare(ctx, tx, r.Credentials, v, snapshot)
		if err != nil {
			return Proposal{}, err
		}
		return s.initial(ctx, tx, v, snapshot, refs)
	})
}
