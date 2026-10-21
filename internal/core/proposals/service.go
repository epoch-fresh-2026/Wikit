package proposals

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type Tx interface {
	content.ReferenceReader
	LockPage(context.Context, string) error
	LockProposal(context.Context, string) error
	Proposal(context.Context, string) (Proposal, error)
	InsertInitial(context.Context, Proposal, []content.MediaRef) error
	InsertCandidate(context.Context, Proposal, []content.MediaRef) error
	SaveProposal(context.Context, Proposal) error
	BlobOnPage(context.Context, string, string) (bool, error)
	Publish(context.Context, content.Published, []content.MediaRef) error
	Outbox(context.Context, content.Published) error
	Audit(context.Context, identity.Actor, string, string, string) error
	Replay(context.Context, string, string, string, string) ([]byte, error)
	Remember(context.Context, string, string, string, string, []byte) error
	Cursor(context.Context, string, string, string, int) (int, error)
	NewCursor(context.Context, string, string, string, int, int) (string, error)
}
type Repository interface {
	WithinProposals(context.Context, func(Tx) error) error
}
type Service struct {
	repo    Repository
	secrets identity.Secrets
	text    content.Text
	lines   Lines
	files   content.Originals
}

func New(repo Repository, secrets identity.Secrets, text content.Text, lines Lines, files content.Originals) *Service {
	return &Service{repo, secrets, text, lines, files}
}

type Expected struct {
	Head      *string `json:"expected_head_revision_id"`
	Candidate string  `json:"expected_proposal_revision_id"`
}

func canonicalID(id *string) *string {
	if id == nil {
		return nil
	}
	v := strings.ToLower(*id)
	return &v
}
func same(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func (e Expected) check(p Proposal, page content.Page, requireBase bool) error {
	e.Head = canonicalID(e.Head)
	if e.Head != nil && !identity.ValidID(*e.Head) || !identity.ValidID(e.Candidate) {
		return fault.Invalid
	}
	if !same(e.Head, page.HeadRevisionID) || strings.ToLower(e.Candidate) != p.CurrentRevision.ID || requireBase && !same(p.CurrentRevision.BaseRevisionID, page.HeadRevisionID) {
		return fault.Stale
	}
	return nil
}

type loaded struct {
	actor    identity.Actor
	policy   access.Policy
	page     content.Page
	proposal Proposal
}

// write owns the full authorization, lock, replay, mutation and commit boundary.
// Replay requires current read access even when a prior write permission was revoked.
func write[T any](ctx context.Context, s *Service, r access.Request, id, op string, pageScope bool, fn func(Tx, loaded) (T, error)) (T, error) {
	var out T
	id = strings.ToLower(id)
	if (pageScope && !access.ValidPageID(id)) || (!pageScope && !identity.ValidID(id)) {
		return out, fault.NotFound
	}
	if len(r.Key) < 8 || len(r.Key) > 128 || r.Hash == "" || r.ID == "" {
		return out, fault.BadRequest
	}
	r.Credentials.Write = true
	err := s.repo.WithinProposals(ctx, func(tx Tx) error {
		a, err := identity.Require(ctx, tx, r.Credentials)
		if err != nil {
			return err
		}
		v := loaded{actor: a}
		pageID := id
		if !pageScope {
			p, err := tx.Proposal(ctx, id)
			if err != nil {
				return err
			}
			pageID = p.PageID
		}
		if err := tx.LockPage(ctx, pageID); err != nil {
			return err
		}
		if !pageScope {
			if err := tx.LockProposal(ctx, id); err != nil {
				return err
			}
			v.proposal, err = tx.Proposal(ctx, id)
			if err != nil {
				return err
			}
		}
		_, v.policy, err = access.Authorize(ctx, tx, r.Credentials, pageID, "read", false)
		if err != nil {
			return err
		}
		v.page, err = tx.Page(ctx, pageID)
		if err != nil {
			return err
		}
		operation := op + ":" + id
		raw, err := tx.Replay(ctx, a.ID, operation, r.Key, r.Hash)
		if err != nil {
			return err
		}
		if raw != nil {
			return json.Unmarshal(raw, &out)
		}
		out, err = fn(tx, v)
		if err != nil {
			return err
		}
		if err := tx.Audit(ctx, a, op, id, r.ID); err != nil {
			return err
		}
		raw, err = json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Remember(ctx, a.ID, operation, r.Key, r.Hash, raw)
	})
	return out, err
}
func permission(ctx context.Context, tx Tx, v loaded, cap string) error {
	groups, err := tx.Groups(ctx)
	if err != nil {
		return err
	}
	return access.CheckPolicy(v.actor, v.policy, groups, cap, false)
}
func editor(ctx context.Context, tx Tx, v loaded) error {
	if v.proposal.AuthorID == v.actor.ID {
		return nil
	}
	return permission(ctx, tx, v, "manage")
}
func open(p Proposal) error {
	if p.State != "open" {
		return fault.State
	}
	return nil
}
func (s *Service) prepare(ctx context.Context, tx Tx, c identity.Credentials, v loaded, in content.Snapshot) (content.Snapshot, []content.MediaRef, error) {
	out, refs, err := content.Normalize(s.text, in)
	if err != nil {
		return out, nil, err
	}
	if out.Type != v.page.Type {
		return out, nil, fault.Invalid
	}
	if out.BlobID != nil {
		b, err := tx.Blob(ctx, *out.BlobID)
		if err != nil {
			return out, nil, err
		}
		if v.actor.Role != "admin" && b.Uploader != v.actor.ID {
			used, err := tx.BlobOnPage(ctx, v.page.ID, b.ID)
			if err != nil {
				return out, nil, err
			}
			if !used {
				return out, nil, fault.NotFound
			}
		}
		if b.State != "ready" {
			return out, nil, fault.State
		}
		if err := s.files.Check(ctx, b); err != nil {
			return out, nil, err
		}
	}
	err = content.CheckReferences(ctx, tx, c, refs, s.files)
	return out, refs, err
}
func (s *Service) initial(ctx context.Context, tx Tx, v loaded, snapshot content.Snapshot, refs []content.MediaRef) (Proposal, error) {
	id, err := s.secrets.ID()
	if err != nil {
		return Proposal{}, err
	}
	candidate, err := s.secrets.ID()
	if err != nil {
		return Proposal{}, err
	}
	p := Initial(id, v.page.ID, v.actor.ID, candidate, snapshot, time.Now().UTC())
	p.CurrentRevision.BaseRevisionID = v.page.HeadRevisionID
	return p, tx.InsertInitial(ctx, p, refs)
}
func (s *Service) Create(ctx context.Context, r access.Request, page string, head *string, in content.Snapshot) (Proposal, error) {
	return write(ctx, s, r, page, "proposal.create", true, func(tx Tx, v loaded) (Proposal, error) {
		if err := permission(ctx, tx, v, "propose"); err != nil {
			return Proposal{}, err
		}
		head = canonicalID(head)
		if head != nil && !identity.ValidID(*head) {
			return Proposal{}, fault.Invalid
		}
		if !same(head, v.page.HeadRevisionID) {
			return Proposal{}, fault.Stale
		}
		snapshot, refs, err := s.prepare(ctx, tx, r.Credentials, v, in)
		if err != nil {
			return Proposal{}, err
		}
		return s.initial(ctx, tx, v, snapshot, refs)
	})
}
func (s *Service) candidate(ctx context.Context, tx Tx, v loaded, snapshot content.Snapshot, refs []content.MediaRef) (Proposal, error) {
	p := v.proposal
	id, err := s.secrets.ID()
	if err != nil {
		return p, err
	}
	p.CurrentRevision = Revision{ID: id, ProposalID: p.ID, BaseRevisionID: v.page.HeadRevisionID, Snapshot: snapshot, CreatedAt: time.Now().UTC()}
	p.Version++
	if err := tx.InsertCandidate(ctx, p, refs); err != nil {
		return p, err
	}
	return p, tx.SaveProposal(ctx, p)
}
func (s *Service) Update(ctx context.Context, r access.Request, id string, e Expected, in content.Snapshot) (Proposal, error) {
	return write(ctx, s, r, id, "proposal.update", false, func(tx Tx, v loaded) (Proposal, error) {
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
		snapshot, refs, err := s.prepare(ctx, tx, r.Credentials, v, in)
		if err != nil {
			return Proposal{}, err
		}
		return s.candidate(ctx, tx, v, snapshot, refs)
	})
}
func (s *Service) State(ctx context.Context, r access.Request, id, state string, version int64) (Proposal, error) {
	return write(ctx, s, r, id, "proposal.state", false, func(tx Tx, v loaded) (Proposal, error) {
		p := v.proposal
		if !slices.Contains([]string{"open", "closed"}, state) || version < 0 {
			return p, fault.Invalid
		}
		if p.State == "merged" || p.State == state {
			return p, fault.State
		}
		if err := editor(ctx, tx, v); err != nil {
			return p, err
		}
		if version != p.Version {
			return p, fault.Stale
		}
		if state == "open" {
			if err := permission(ctx, tx, v, "propose"); err != nil {
				return p, err
			}
		}
		p.State = state
		p.Version++
		return p, tx.SaveProposal(ctx, p)
	})
}
func (s *Service) Merge(ctx context.Context, r access.Request, id string, e Expected) (content.Published, error) {
	return write(ctx, s, r, id, "proposal.merge", false, func(tx Tx, v loaded) (content.Published, error) {
		if err := permission(ctx, tx, v, "merge"); err != nil {
			return content.Published{}, err
		}
		if err := open(v.proposal); err != nil {
			return content.Published{}, err
		}
		if err := e.check(v.proposal, v.page, true); err != nil {
			return content.Published{}, err
		}
		snapshot, refs, err := s.prepare(ctx, tx, r.Credentials, v, v.proposal.CurrentRevision.Snapshot)
		if err != nil {
			return content.Published{}, err
		}
		id, err := s.secrets.ID()
		if err != nil {
			return content.Published{}, err
		}
		out := content.Published{PublisherID: &v.actor.ID, ID: id, PageID: v.page.ID, ParentID: v.page.HeadRevisionID, SourceProposalID: &v.proposal.ID, SourceKind: "merge", Snapshot: snapshot, CreatedAt: time.Now().UTC(), AuthorID: &v.proposal.AuthorID}
		if snapshot.Type == "media" {
			url := "/api/v1/pages/" + out.PageID + "/revisions/" + id + "/content"
			out.ContentURL = &url
		}
		if err := tx.Publish(ctx, out, refs); err != nil {
			return out, err
		}
		p := v.proposal
		p.State = "merged"
		p.Version++
		if err := tx.SaveProposal(ctx, p); err != nil {
			return out, err
		}
		return out, tx.Outbox(ctx, out)
	})
}
