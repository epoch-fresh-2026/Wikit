package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/jackc/pgx/v5"
)

func (s *Store) WithinAccess(ctx context.Context, fn func(access.Tx) error) error {
	return s.withinSecurity(ctx, func(tx *securityTx) error { return fn(tx) })
}
func (t *securityTx) ActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := t.QueryRow(ctx, "SELECT count(*) FROM wikit.users WHERE role='admin' AND status='active'").Scan(&n)
	return n, storageError(err)
}
func (t *securityTx) Accounts(ctx context.Context) ([]access.AccountAdmin, error) {
	rows, err := t.Query(ctx, "SELECT id::text,username,display_name,role,status,auth_version,create_page FROM wikit.users ORDER BY id")
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []access.AccountAdmin{}
	for rows.Next() {
		var v access.AccountAdmin
		if err := rows.Scan(&v.Account.ID, &v.Account.Username, &v.Account.DisplayName, &v.Account.Role, &v.Account.Status, &v.AuthVersion, &v.Account.CreatePage); err != nil {
			return nil, storageError(err)
		}
		out = append(out, v)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) Groups(ctx context.Context) ([]access.Group, error) {
	rows, err := t.Query(ctx, `SELECT g.id::text,g.name,g.version,g.create_page,coalesce(array_agg(m.user_id::text ORDER BY m.user_id) FILTER(WHERE m.user_id IS NOT NULL),'{}') FROM wikit.groups g LEFT JOIN wikit.group_members m ON m.group_id=g.id GROUP BY g.id ORDER BY g.id`)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []access.Group{}
	for rows.Next() {
		var g access.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Version, &g.CreatePage, &g.Members); err != nil {
			return nil, storageError(err)
		}
		out = append(out, g)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) Group(ctx context.Context, id string) (access.Group, error) {
	groups, err := t.Groups(ctx)
	if err != nil {
		return access.Group{}, err
	}
	for _, g := range groups {
		if g.ID == id {
			return g, nil
		}
	}
	return access.Group{}, fault.NotFound
}
func (t *securityTx) SaveGroup(ctx context.Context, g access.Group) error {
	if _, err := t.Exec(ctx, `INSERT INTO wikit.groups VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET name=excluded.name,version=excluded.version,create_page=excluded.create_page`, g.ID, g.Name, g.Version, g.CreatePage); err != nil {
		return storageError(err)
	}
	if _, err := t.Exec(ctx, "DELETE FROM wikit.group_members WHERE group_id=$1", g.ID); err != nil {
		return storageError(err)
	}
	for _, id := range g.Members {
		if _, err := t.Exec(ctx, "INSERT INTO wikit.group_members VALUES($1,$2)", g.ID, id); err != nil {
			return storageError(err)
		}
	}
	return nil
}
func (t *securityTx) Policy(ctx context.Context, id string) (access.Policy, error) {
	var p access.Policy
	err := t.QueryRow(ctx, "SELECT page_id::text,visibility,status,policy_version FROM wikit.page_policies WHERE page_id=$1", id).Scan(&p.PageID, &p.Visibility, &p.Status, &p.Version)
	if err != nil {
		return p, storageError(err)
	}
	rows, err := t.Query(ctx, "SELECT subject_type,subject_id::text,array_agg(capability ORDER BY capability) FROM wikit.page_grants WHERE page_id=$1 GROUP BY subject_type,subject_id ORDER BY subject_type,subject_id", id)
	if err != nil {
		return p, storageError(err)
	}
	defer rows.Close()
	p.Grants = []access.Grant{}
	for rows.Next() {
		var g access.Grant
		if err := rows.Scan(&g.SubjectType, &g.SubjectID, &g.Capabilities); err != nil {
			return p, storageError(err)
		}
		p.Grants = append(p.Grants, g)
	}
	return p, storageError(rows.Err())
}
func (t *securityTx) SavePolicy(ctx context.Context, p access.Policy) error {
	if _, err := t.Exec(ctx, `INSERT INTO wikit.page_policies VALUES($1,$2,$3,$4) ON CONFLICT(page_id) DO UPDATE SET visibility=excluded.visibility,status=excluded.status,policy_version=excluded.policy_version`, p.PageID, p.Visibility, p.Status, p.Version); err != nil {
		return storageError(err)
	}
	if _, err := t.Exec(ctx, "DELETE FROM wikit.page_grants WHERE page_id=$1", p.PageID); err != nil {
		return storageError(err)
	}
	for _, g := range p.Grants {
		for _, c := range g.Capabilities {
			if _, err := t.Exec(ctx, "INSERT INTO wikit.page_grants VALUES($1,$2,$3,$4)", p.PageID, g.SubjectType, g.SubjectID, c); err != nil {
				return storageError(err)
			}
		}
	}
	return nil
}
func (t *securityTx) Audits(ctx context.Context) ([]identity.Audit, error) {
	rows, err := t.Query(ctx, "SELECT id::text,actor_id::text,action,target_id::text,request_id,created_at FROM wikit.audit_events ORDER BY created_at,id")
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []identity.Audit{}
	for rows.Next() {
		var a identity.Audit
		if err := rows.Scan(&a.ID, &a.ActorID, &a.Action, &a.TargetID, &a.RequestID, &a.CreatedAt); err != nil {
			return nil, storageError(err)
		}
		out = append(out, a)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) Replay(ctx context.Context, actor, operation, key, hash string) ([]byte, error) {
	var previous string
	var result []byte
	err := t.QueryRow(ctx, "SELECT request_hash,result FROM wikit.idempotency_records WHERE actor_id=$1 AND operation=$2 AND key=$3 AND expires_at>now()", actor, operation, key).Scan(&previous, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, storageError(err)
	}
	if !access.SameRequestHash(previous, hash) {
		return nil, fault.Idempotency
	}
	return result, nil
}
func (t *securityTx) Remember(ctx context.Context, actor, operation, key, hash string, result []byte) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.idempotency_records VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(actor_id,operation,key) DO UPDATE SET request_hash=excluded.request_hash,result=excluded.result,expires_at=excluded.expires_at`, actor, operation, key, hash, result, time.Now().Add(24*time.Hour))
	return storageError(err)
}
func (t *securityTx) Cursor(ctx context.Context, plain, actor, kind string, limit int) (int, error) {
	var offset int
	err := t.QueryRow(ctx, "SELECT position FROM wikit.list_cursors WHERE digest=$1 AND actor_id IS NOT DISTINCT FROM nullif($2,'')::uuid AND kind=$3 AND page_size=$4 AND expires_at>now()", identity.Digest(plain), actor, kind, limit).Scan(&offset)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fault.BadRequest
	}
	return offset, storageError(err)
}
func (t *securityTx) NewCursor(ctx context.Context, plain, actor, kind string, position, limit int) (string, error) {
	_, err := t.Exec(ctx, "INSERT INTO wikit.list_cursors(digest,actor_id,kind,page_size,position,expires_at) VALUES($1,nullif($2,'')::uuid,$3,$4,$5,$6)", identity.Digest(plain), actor, kind, limit, position, time.Now().Add(10*time.Minute))
	return plain, storageError(err)
}
