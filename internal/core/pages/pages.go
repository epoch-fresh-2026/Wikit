// Package pages owns page creation, publication-history reads and directories.
package pages

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
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
)

type Page = content.Page
type Revision = content.Published
type Detail struct {
	Page         Page      `json:"page"`
	Revision     *Revision `json:"revision"`
	Capabilities []string  `json:"capabilities"`
}
type Created struct {
	Page     Page     `json:"page"`
	Revision Revision `json:"revision"`
}
type Blob = content.Blob
type Tx interface {
	access.Authorization
	SavePolicy(context.Context, access.Policy) error
	InsertPage(context.Context, Page) error
	NextPageID(context.Context) (string, error)
	Publish(context.Context, content.Published, []content.MediaRef) error
	Outbox(context.Context, content.Published) error
	Page(context.Context, string) (Page, error)
	PageCandidates(context.Context, string, string) ([]Page, error)
	PageBatch(context.Context, content.TagFilter, string, string, int) ([]Page, error)
	PageDetails(context.Context, []string) ([]Detail, error)
	PageAnchor(context.Context, string, string, string, int) (string, error)
	NewPageCursor(context.Context, string, string, string, string, int) (string, error)
	Policies(context.Context, []string) ([]access.Policy, error)
	Revision(context.Context, string, string) (Revision, error)
	RevisionIDs(context.Context, string) ([]string, error)
	PublishedTags(context.Context, []string) ([]string, error)
	Proposal(context.Context, string) (proposals.Proposal, error)
	ProposalCandidates(context.Context, string, string) ([]proposals.Summary, error)
	CandidateIDs(context.Context, string) ([]string, error)
	Candidate(context.Context, string, string) (proposals.Revision, error)
	Blob(context.Context, string) (Blob, error)
	Audit(context.Context, identity.Actor, string, string, string) error
	Moderation(context.Context, access.Moderation) error
	Replay(context.Context, string, string, string, string) ([]byte, error)
	Remember(context.Context, string, string, string, string, []byte) error
	Cursor(context.Context, string, string, string, int) (int, error)
	NewCursor(context.Context, string, string, string, int, int) (string, error)
}
type Repository interface {
	WithinPages(context.Context, func(Tx) error) error
	Allow(context.Context, string, string) error
}
type Service struct {
	repo    Repository
	secrets identity.Secrets
	text    content.Text
	files   content.Originals
}

func New(repo Repository, secrets identity.Secrets, text content.Text, files content.Originals) *Service {
	return &Service{repo, secrets, text, files}
}
func (s *Service) Create(ctx context.Context, r access.Request, visibility string, snapshot content.Snapshot, ip string) (Created, error) {
	if visibility != "public" && visibility != "restricted" {
		return Created{}, fault.Invalid
	}
	r.Credentials.Write = true
	if len(r.Key) < 8 || len(r.Key) > 128 || r.Hash == "" || r.ID == "" {
		return Created{}, fault.BadRequest
	}
	var actorID string
	if err := s.repo.WithinPages(ctx, func(tx Tx) error { a, err := identity.Require(ctx, tx, r.Credentials); actorID = a.ID; return err }); err != nil {
		return Created{}, err
	}
	if err := s.repo.Allow(ctx, "page.create", ip+"\x00"+actorID); err != nil {
		return Created{}, err
	}
	snapshot, refs, err := content.Normalize(s.text, snapshot)
	if err != nil {
		return Created{}, err
	}
	var out Created
	err = s.repo.WithinPages(ctx, func(tx Tx) error {
		a, err := identity.Require(ctx, tx, r.Credentials)
		if err != nil {
			return err
		}
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		if !access.CanCreate(a, groups) {
			return fault.Forbidden
		}
		previous, err := tx.Replay(ctx, a.ID, "page.create", r.Key, r.Hash)
		if err != nil {
			return err
		}
		if previous != nil {
			if err := json.Unmarshal(previous, &out); err != nil {
				return err
			}
			_, _, err = access.Authorize(ctx, tx, r.Credentials, out.Page.ID, "read", false)
			return err
		}
		if snapshot.Type == "media" {
			b, err := tx.Blob(ctx, *snapshot.BlobID)
			if err != nil {
				return err
			}
			if a.Role != "admin" && b.Uploader != a.ID {
				return fault.NotFound
			}
			if b.State != "ready" {
				return fault.State
			}
			if err := s.files.Check(ctx, b); err != nil {
				return err
			}
		}
		if err := content.CheckReferences(ctx, tx, r.Credentials, refs, s.files); err != nil {
			return err
		}
		id, err := tx.NextPageID(ctx)
		if err != nil {
			return err
		}
		revisionID, err := s.secrets.ID()
		if err != nil {
			return err
		}
		out.Page = Page{ID: id, Type: snapshot.Type, Visibility: visibility, Status: "published", PolicyVersion: 1}
		policy := access.Policy{PageID: id, Visibility: visibility, Status: "published", Version: 1, Grants: []access.Grant{{SubjectType: "user", SubjectID: a.ID, Capabilities: []string{"read", "propose", "discuss"}}}}
		if err := tx.SavePolicy(ctx, policy); err != nil {
			return err
		}
		if err := tx.InsertPage(ctx, out.Page); err != nil {
			return err
		}
		out.Revision = Revision{PublisherID: &a.ID, ID: revisionID, PageID: id, Snapshot: snapshot, AuthorID: &a.ID, SourceKind: "create", CreatedAt: time.Now().UTC()}
		if snapshot.Type == "media" {
			url := "/api/v1/pages/" + id + "/revisions/" + revisionID + "/content"
			out.Revision.ContentURL = &url
		}
		if err := tx.Publish(ctx, out.Revision, refs); err != nil {
			return err
		}
		if err := tx.Outbox(ctx, out.Revision); err != nil {
			return err
		}
		out.Page.HeadRevisionID = &revisionID
		if err := tx.Audit(ctx, a, "page.create", id, r.ID); err != nil {
			return err
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Remember(ctx, a.ID, "page.create", r.Key, r.Hash, encoded)
	})
	return out, err
}
func detail(ctx context.Context, tx Tx, id string) (Detail, error) {
	p, err := tx.Page(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	out := Detail{Page: p, Capabilities: []string{}}
	if p.HeadRevisionID != nil {
		v, err := tx.Revision(ctx, id, *p.HeadRevisionID)
		if err != nil {
			return Detail{}, err
		}
		out.Revision = &v
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, c identity.Credentials, id, revisionID string) (any, error) {
	id = strings.ToLower(id)
	revisionID = strings.ToLower(revisionID)
	if !access.ValidPageID(id) || (revisionID != "" && !identity.ValidID(revisionID)) {
		return nil, fault.NotFound
	}
	var out any
	err := s.repo.WithinPages(ctx, func(tx Tx) error {
		actor, policy, err := access.Authorize(ctx, tx, c, id, "read", false)
		if err != nil {
			return err
		}
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		if revisionID != "" {
			out, err = tx.Revision(ctx, id, revisionID)
		} else {
			d, e := detail(ctx, tx, id)
			err = e
			d.Capabilities = access.Capabilities(actor, policy, groups)
			out = d
		}
		return err
	})
	return out, err
}
func (s *Service) Proposal(ctx context.Context, c identity.Credentials, id string) (proposals.Proposal, error) {
	id = strings.ToLower(id)
	if !identity.ValidID(id) {
		return proposals.Proposal{}, fault.NotFound
	}
	var out proposals.Proposal
	err := s.repo.WithinPages(ctx, func(tx Tx) error {
		p, err := tx.Proposal(ctx, id)
		if err != nil {
			return err
		}
		if _, _, err := access.Authorize(ctx, tx, c, p.PageID, "read", false); err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

type Query struct {
	Kind, PageID, ProposalID, Tag, Type, State, Cursor string
	TagSet                                             bool
	Filter                                             content.TagFilter
	Limit                                              int
}
type List struct {
	Items []json.RawMessage `json:"items"`
	Next  *string           `json:"next_cursor"`
}

func (s *Service) List(ctx context.Context, c identity.Credentials, q Query) (List, error) {
	q.PageID = strings.ToLower(q.PageID)
	q.ProposalID = strings.ToLower(q.ProposalID)
	if q.Limit < 1 || q.Limit > 100 || !slices.Contains([]string{"pages", "revisions", "proposals", "candidates", "tags"}, q.Kind) || (q.Type != "" && q.Type != "document" && q.Type != "media") || (q.State != "" && !slices.Contains([]string{"open", "closed", "merged"}, q.State)) {
		return List{}, fault.BadRequest
	}
	if q.PageID != "" && !access.ValidPageID(q.PageID) || q.ProposalID != "" && !identity.ValidID(q.ProposalID) {
		return List{}, fault.NotFound
	}
	var err error
	q.Filter, err = q.Filter.Normalize(s.text, q.Tag, q.TagSet)
	if err != nil {
		return List{}, err
	}
	q.Tag = content.TagKey(s.text, q.Tag)
	var out List
	err = s.repo.WithinPages(ctx, func(tx Tx) error {
		a, err := identity.Resolve(ctx, tx, c)
		if err != nil {
			return err
		}
		groups, err := tx.Groups(ctx)
		if err != nil {
			return err
		}
		if q.ProposalID != "" {
			p, err := tx.Proposal(ctx, q.ProposalID)
			if err != nil {
				return err
			}
			q.PageID = p.PageID
		}
		if q.PageID != "" {
			if _, _, err := access.Authorize(ctx, tx, c, q.PageID, "read", false); err != nil {
				return err
			}
		}
		// Keep unrelated history, proposal and tag-list cursors compatible.
		var bindingBytes []byte
		if q.Kind == "pages" {
			bindingBytes, err = json.Marshal(struct {
				Kind, Type string
				Filter     content.TagFilter
			}{q.Kind, q.Type, q.Filter})
		} else {
			bindingBytes, err = json.Marshal(struct{ Kind, Page, Proposal, Tag, Type, State string }{q.Kind, q.PageID, q.ProposalID, q.Tag, q.Type, q.State})
		}
		if err != nil {
			return err
		}
		binding := "content:" + identity.Digest(string(bindingBytes))
		if q.Kind == "pages" {
			out, err = s.directory(ctx, tx, a, groups, q, binding)
			return err
		}
		offset := 0
		if q.Cursor != "" {
			offset, err = tx.Cursor(ctx, q.Cursor, a.ID, binding, q.Limit)
			if err != nil {
				return err
			}
		}
		ids := []string{}
		policies := map[string]access.Policy{}
		loadPolicies := func(ids []string) error {
			if len(ids) == 0 {
				return nil
			}
			rows, err := tx.Policies(ctx, ids)
			if err != nil {
				return err
			}
			for _, p := range rows {
				policies[p.PageID] = p
			}
			return nil
		}
		readable := func(p Page) (bool, error) {
			policy := access.Policy{PageID: p.ID, Visibility: p.Visibility, Status: p.Status}
			if p.Visibility == "restricted" {
				var ok bool
				policy, ok = policies[p.ID]
				if !ok {
					return false, fault.NotFound
				}
			}
			err := access.CheckPolicy(a, policy, groups, "read", false)
			if err == fault.NotFound || err == fault.Forbidden {
				return false, nil
			}
			return err == nil, err
		}
		switch q.Kind {
		case "tags":
			rows, err := tx.PageCandidates(ctx, q.Tag, q.Type)
			if err != nil {
				return err
			}
			restricted := []string{}
			for _, p := range rows {
				if p.Status == "published" && p.HeadRevisionID != nil && p.Visibility == "restricted" {
					restricted = append(restricted, p.ID)
				}
			}
			if err := loadPolicies(restricted); err != nil {
				return err
			}
			heads := []string{}
			for _, p := range rows {
				if p.Status != "published" || p.HeadRevisionID == nil {
					continue
				}
				ok, err := readable(p)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				heads = append(heads, *p.HeadRevisionID)
			}
			ids, err = tx.PublishedTags(ctx, heads)
			if err != nil {
				return err
			}
		case "revisions":
			if q.PageID == "" {
				return fault.BadRequest
			}
			ids, err = tx.RevisionIDs(ctx, q.PageID)
			if err != nil {
				return err
			}
		case "proposals":
			rows, err := tx.ProposalCandidates(ctx, q.PageID, q.State)
			if err != nil {
				return err
			}
			pageIDs := []string{}
			for _, p := range rows {
				pageIDs = append(pageIDs, p.PageID)
			}
			if err := loadPolicies(pageIDs); err != nil {
				return err
			}
			for _, p := range rows {
				policy, ok := policies[p.PageID]
				if !ok {
					return fault.NotFound
				}
				if access.CheckPolicy(a, policy, groups, "read", false) == nil {
					ids = append(ids, p.ID)
				}
			}
		case "candidates":
			if q.ProposalID == "" {
				return fault.BadRequest
			}
			ids, err = tx.CandidateIDs(ctx, q.ProposalID)
			if err != nil {
				return err
			}
		}
		if offset > len(ids) {
			return fault.BadRequest
		}
		end := min(offset+q.Limit, len(ids))
		out.Items = []json.RawMessage{}
		for _, id := range ids[offset:end] {
			var v any
			var err error
			switch q.Kind {
			case "revisions":
				v, err = tx.Revision(ctx, q.PageID, id)
			case "proposals":
				v, err = tx.Proposal(ctx, id)
			case "candidates":
				v, err = tx.Candidate(ctx, q.ProposalID, id)
			case "tags":
				v = struct {
					Key string `json:"key"`
				}{id}
			}
			if err != nil {
				return err
			}
			raw, err := json.Marshal(v)
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
			token, err := tx.NewCursor(ctx, plain, a.ID, binding, end, q.Limit)
			if err != nil {
				return err
			}
			out.Next = &token
		}
		return nil
	})
	return out, err
}
