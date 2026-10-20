// Package access owns page capabilities and account/group governance.
package access

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func capabilities() []string { return []string{"read", "propose", "discuss", "review", "manage"} }

type Grant struct {
	SubjectType  string   `json:"subject_type"`
	SubjectID    string   `json:"subject_id"`
	Capabilities []string `json:"capabilities"`
}
type Policy struct {
	PageID     string  `json:"-"`
	Visibility string  `json:"visibility"`
	Status     string  `json:"-"`
	Version    int64   `json:"policy_version"`
	Grants     []Grant `json:"grants"`
}

// Moderation is the immutable governance decision saved with its protected mutation.
type Moderation struct {
	Actor                                     identity.Actor
	PageID, TargetID, Kind, Reason, RequestID string
	Hidden                                    bool
	Version                                   int64
}
type Group struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Version int64    `json:"version"`
	Members []string `json:"member_ids"`
}
type AccountAdmin struct {
	Account     identity.Account `json:"account"`
	AuthVersion int64            `json:"auth_version"`
}

// Authorization is the small consumer view future protected use cases require.
type Authorization interface {
	identity.Authentication
	Policy(context.Context, string) (Policy, error)
	Groups(context.Context) ([]Group, error)
}

// Governance transactions share current credential resolution with identity.
type Tx interface {
	Authorization
	SaveUser(context.Context, identity.Record) error
	ActiveAdmins(context.Context) (int, error)
	Accounts(context.Context) ([]AccountAdmin, error)
	Group(context.Context, string) (Group, error)
	SaveGroup(context.Context, Group) error
	SavePolicy(context.Context, Policy) error
	Audit(context.Context, identity.Actor, string, string, string) error
	Audits(context.Context) ([]identity.Audit, error)
	Replay(context.Context, string, string, string, string) ([]byte, error)
	Remember(context.Context, string, string, string, string, []byte) error
	Cursor(context.Context, string, string, string, int) (int, error)
	NewCursor(context.Context, string, string, string, int, int) (string, error)
}
type Repository interface {
	WithinAccess(context.Context, func(Tx) error) error
}
type Service struct {
	repo    Repository
	secrets identity.Secrets
}

func New(repo Repository, secrets identity.Secrets) *Service { return &Service{repo, secrets} }

type Request struct {
	Credentials   identity.Credentials
	ID, Key, Hash string
}

func effective(a identity.Actor, p Policy, groups []Group) map[string]bool {
	caps := map[string]bool{}
	if a.ID != "" && a.Status != "active" {
		return caps
	}
	if a.Role == "admin" {
		for _, c := range capabilities() {
			caps[c] = true
		}
		return caps
	}
	member := map[string]bool{}
	for _, g := range groups {
		if slices.Contains(g.Members, a.ID) && a.ID != "" {
			member[g.ID] = true
		}
	}
	for _, g := range p.Grants {
		if (g.SubjectType == "user" && g.SubjectID == a.ID && a.ID != "") || (g.SubjectType == "group" && member[g.SubjectID]) {
			for _, c := range g.Capabilities {
				caps[c] = true
			}
		}
	}
	if p.Status == "hidden" && !caps["manage"] {
		return map[string]bool{}
	}
	if p.Status != "hidden" && p.Visibility == "public" {
		caps["read"] = true
		if a.ID != "" {
			caps["propose"] = true
			caps["discuss"] = true
			if a.Role == "reviewer" {
				caps["review"] = true
			}
		}
	}
	return caps
}

// Authorize is shared by all consumers, including future history/download and
// publication use cases. The supplied tx must also own their protected write.
func Authorize(ctx context.Context, tx Authorization, c identity.Credentials, pageID, capability string, management bool) (identity.Actor, Policy, error) {
	pageID = strings.ToLower(pageID)
	a, err := identity.Resolve(ctx, tx, c)
	if err != nil {
		return a, Policy{}, err
	}
	p, err := tx.Policy(ctx, pageID)
	if err != nil {
		return a, p, err
	}
	groups, err := tx.Groups(ctx)
	if err != nil {
		return a, p, err
	}
	if err := CheckPolicy(a, p, groups, capability, management); err != nil {
		return a, p, err
	}
	return a, p, nil
}

// CheckPolicy evaluates a current policy within an already authenticated read
// transaction; directory consumers can reuse its account/group view.
func CheckPolicy(a identity.Actor, p Policy, groups []Group, capability string, management bool) error {
	caps := effective(a, p, groups)
	if !caps["read"] || (p.Status == "hidden" && !management) {
		return fault.NotFound
	}
	if !caps[capability] {
		return fault.Forbidden
	}
	return nil
}
func (s *Service) Check(ctx context.Context, c identity.Credentials, pageID, capability string, management bool) error {
	return s.repo.WithinAccess(ctx, func(tx Tx) error { _, _, err := Authorize(ctx, tx, c, pageID, capability, management); return err })
}
func admin(ctx context.Context, tx Tx, c identity.Credentials) (identity.Actor, error) {
	a, err := identity.Require(ctx, tx, c)
	if err != nil {
		return a, err
	}
	if a.Role != "admin" {
		return a, fault.Forbidden
	}
	return a, nil
}
func (s *Service) GetPolicy(ctx context.Context, c identity.Credentials, id string) (Policy, error) {
	var out Policy
	err := s.repo.WithinAccess(ctx, func(tx Tx) error { _, p, err := Authorize(ctx, tx, c, id, "manage", true); out = p; return err })
	return out, err
}

// mutate keeps authorization, idempotency, state and audit in one transaction.
// check is executed even on replay so revoked privileges cannot retrieve results.
func (s *Service) mutate(ctx context.Context, r Request, operation string, check func(Tx) (identity.Actor, error), apply func(Tx, identity.Actor) (any, string, error)) ([]byte, error) {
	if len(r.Key) < 8 || len(r.Key) > 128 || r.Hash == "" || r.ID == "" {
		return nil, fault.BadRequest
	}
	var out []byte
	err := s.repo.WithinAccess(ctx, func(tx Tx) error {
		a, err := check(tx)
		if err != nil {
			return err
		}
		previous, err := tx.Replay(ctx, a.ID, operation, r.Key, r.Hash)
		if err != nil {
			return err
		}
		if previous != nil {
			out = previous
			return nil
		}
		value, target, err := apply(tx, a)
		if err != nil {
			return err
		}
		out, err = json.Marshal(value)
		if err != nil {
			return err
		}
		if err := tx.Audit(ctx, a, operation, target, r.ID); err != nil {
			return err
		}
		return tx.Remember(ctx, a.ID, operation, r.Key, r.Hash, out)
	})
	return out, err
}
func (s *Service) UpdateAccount(ctx context.Context, r Request, id, role, status string, version int64) ([]byte, error) {
	id = strings.ToLower(id)
	r.Credentials.Write = true
	if !identity.ValidID(id) || !slices.Contains([]string{"member", "reviewer", "admin"}, role) || !slices.Contains([]string{"active", "disabled"}, status) {
		return nil, fault.Invalid
	}
	return s.mutate(ctx, r, "account.update:"+id, func(tx Tx) (identity.Actor, error) { return admin(ctx, tx, r.Credentials) }, func(tx Tx, a identity.Actor) (any, string, error) {
		u, err := tx.User(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if u.AuthVersion != version {
			return nil, "", fault.Stale
		}
		if u.Role == "admin" && u.Status == "active" && (role != "admin" || status != "active") {
			n, err := tx.ActiveAdmins(ctx)
			if err != nil {
				return nil, "", err
			}
			if n <= 1 {
				return nil, "", fault.State
			}
		}
		u.Role = role
		u.Status = status
		u.AuthVersion++
		if err := tx.SaveUser(ctx, u); err != nil {
			return nil, "", err
		}
		return AccountAdmin{u.Account, u.AuthVersion}, id, nil
	})
}
func (s *Service) SaveGroup(ctx context.Context, r Request, id, name string, members []string, version int64) ([]byte, error) {
	id = strings.ToLower(id)
	members = append([]string(nil), members...)
	for i := range members {
		members[i] = strings.ToLower(members[i])
	}
	r.Credentials.Write = true
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || len(members) > 10000 {
		return nil, fault.Invalid
	}
	creating := id == ""
	if creating {
		var err error
		id, err = s.secrets.ID()
		if err != nil {
			return nil, err
		}
	} else if !identity.ValidID(id) {
		return nil, fault.Invalid
	}
	op := "group.update:" + id
	if creating {
		op = "group.create"
	}
	return s.mutate(ctx, r, op, func(tx Tx) (identity.Actor, error) { return admin(ctx, tx, r.Credentials) }, func(tx Tx, a identity.Actor) (any, string, error) {
		if !creating {
			g, err := tx.Group(ctx, id)
			if err != nil {
				return nil, "", err
			}
			if g.Version != version {
				return nil, "", fault.Stale
			}
		}
		seen := map[string]bool{}
		for _, id := range members {
			if !identity.ValidID(id) || seen[id] {
				return nil, "", fault.Invalid
			}
			seen[id] = true
			if _, err := tx.User(ctx, id); err != nil {
				return nil, "", fault.Invalid
			}
		}
		if members == nil {
			members = []string{}
		}
		slices.Sort(members)
		g := Group{id, name, version + 1, members}
		if err := tx.SaveGroup(ctx, g); err != nil {
			return nil, "", err
		}
		return g, id, nil
	})
}
func validatePolicy(ctx context.Context, tx Tx, p Policy) error {
	if !slices.Contains([]string{"public", "restricted"}, p.Visibility) || len(p.Grants) > 1000 {
		return fault.Invalid
	}
	seen := map[string]bool{}
	participant := false
	groups, err := tx.Groups(ctx)
	if err != nil {
		return err
	}
	for _, g := range p.Grants {
		key := g.SubjectType + ":" + g.SubjectID
		if !identity.ValidID(g.SubjectID) || seen[key] || len(g.Capabilities) == 0 {
			return fault.Invalid
		}
		seen[key] = true
		caps := map[string]bool{}
		for _, c := range g.Capabilities {
			if !slices.Contains(capabilities(), c) || caps[c] {
				return fault.Invalid
			}
			caps[c] = true
		}
		if !caps["read"] {
			return fault.Invalid
		}
		var users []string
		if g.SubjectType == "user" {
			users = []string{g.SubjectID}
		} else if g.SubjectType == "group" {
			found := false
			for _, group := range groups {
				if group.ID == g.SubjectID {
					found = true
					users = group.Members
				}
			}
			if !found {
				return fault.Invalid
			}
		} else {
			return fault.Invalid
		}
		for _, id := range users {
			u, err := tx.User(ctx, id)
			if err != nil {
				return fault.Invalid
			}
			if u.Status == "active" && u.Role != "admin" {
				participant = true
			}
		}
	}
	if p.Visibility == "restricted" && !participant {
		return fault.Invalid
	}
	return nil
}
func managedGrants(p Policy) map[string]bool {
	out := map[string]bool{}
	for _, g := range p.Grants {
		if slices.Contains(g.Capabilities, "manage") {
			out[g.SubjectType+":"+g.SubjectID] = true
		}
	}
	return out
}
func (s *Service) UpdatePolicy(ctx context.Context, r Request, id string, proposed Policy, expected int64) ([]byte, error) {
	id = strings.ToLower(id)
	proposed.Grants = append([]Grant(nil), proposed.Grants...)
	for i := range proposed.Grants {
		proposed.Grants[i].SubjectID = strings.ToLower(proposed.Grants[i].SubjectID)
	}
	r.Credentials.Write = true
	if !identity.ValidID(id) {
		return nil, fault.Invalid
	}
	return s.mutate(ctx, r, "policy.update:"+id, func(tx Tx) (identity.Actor, error) {
		a, _, err := Authorize(ctx, tx, r.Credentials, id, "manage", true)
		return a, err
	}, func(tx Tx, a identity.Actor) (any, string, error) {
		_, old, err := Authorize(ctx, tx, r.Credentials, id, "manage", true)
		if err != nil {
			return nil, "", err
		}
		if old.Version != expected {
			return nil, "", fault.Stale
		}
		proposed.PageID = id
		proposed.Status = old.Status
		proposed.Version = old.Version + 1
		if err := validatePolicy(ctx, tx, proposed); err != nil {
			return nil, "", err
		}
		if a.Role != "admin" {
			before, after := managedGrants(old), managedGrants(proposed)
			if len(before) != len(after) {
				return nil, "", fault.Forbidden
			}
			for k := range before {
				if !after[k] {
					return nil, "", fault.Forbidden
				}
			}
			groups, err := tx.Groups(ctx)
			if err != nil {
				return nil, "", err
			}
			allowed := effective(a, old, groups)
			existing := map[string]map[string]bool{}
			for _, g := range old.Grants {
				caps := map[string]bool{}
				for _, c := range g.Capabilities {
					caps[c] = true
				}
				existing[g.SubjectType+":"+g.SubjectID] = caps
			}
			for _, g := range proposed.Grants {
				for _, c := range g.Capabilities {
					if !allowed[c] && !existing[g.SubjectType+":"+g.SubjectID][c] {
						return nil, "", fault.Forbidden
					}
				}
			}
		}
		if proposed.Grants == nil {
			proposed.Grants = []Grant{}
		}
		if err := tx.SavePolicy(ctx, proposed); err != nil {
			return nil, "", err
		}
		return proposed, id, nil
	})
}

// List returns only administrator-visible records. Cursor state is opaque and
// server-side, bound to account, list and page size, with a ten-minute lifetime.
func (s *Service) List(ctx context.Context, c identity.Credentials, kind, cursor string, limit int) (any, error) {
	if limit < 1 || limit > 100 || !slices.Contains([]string{"accounts", "groups", "audit"}, kind) {
		return nil, fault.BadRequest
	}
	var out any
	err := s.repo.WithinAccess(ctx, func(tx Tx) error {
		a, err := admin(ctx, tx, c)
		if err != nil {
			return err
		}
		offset := 0
		if cursor != "" {
			offset, err = tx.Cursor(ctx, cursor, a.ID, kind, limit)
			if err != nil {
				return err
			}
		}
		var all []json.RawMessage
		add := func(value any) error {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			all = append(all, data)
			return nil
		}
		switch kind {
		case "accounts":
			rows, err := tx.Accounts(ctx)
			if err != nil {
				return err
			}
			for _, v := range rows {
				if err := add(v); err != nil {
					return err
				}
			}
		case "groups":
			rows, err := tx.Groups(ctx)
			if err != nil {
				return err
			}
			for _, v := range rows {
				if err := add(v); err != nil {
					return err
				}
			}
		case "audit":
			rows, err := tx.Audits(ctx)
			if err != nil {
				return err
			}
			for _, v := range rows {
				if err := add(v); err != nil {
					return err
				}
			}
		}
		if offset > len(all) {
			return fault.BadRequest
		}
		end := min(offset+limit, len(all))
		items := all[offset:end]
		if items == nil {
			items = []json.RawMessage{}
		}
		var next *string
		if end < len(all) {
			token, err := s.secrets.Secret()
			if err != nil {
				return err
			}
			saved, err := tx.NewCursor(ctx, token, a.ID, kind, end, limit)
			if err != nil {
				return err
			}
			next = &saved
		}
		out = struct {
			Items []json.RawMessage `json:"items"`
			Next  *string           `json:"next_cursor"`
		}{items, next}
		return nil
	})
	return out, err
}
