package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SecurityLock is shared by all protected use cases, including later publication
// transactions. Take it before reading credentials or any authorization state.
const SecurityLock int64 = 8734281002

type securityTx struct{ pgx.Tx }

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fault.NotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fault.Invalid
	}
	return errors.New("security storage unavailable")
}
func (s *Store) withinSecurity(ctx context.Context, fn func(*securityTx) error) (result error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return storageError(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			result = errors.Join(result, errors.New("security rollback failed"))
		}
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", SecurityLock); err != nil {
		return storageError(err)
	}
	if err := fn(&securityTx{tx}); err != nil {
		return err
	}
	return storageError(tx.Commit(ctx))
}
func (s *Store) WithinIdentity(ctx context.Context, fn func(identity.Tx) error) error {
	return s.withinSecurity(ctx, func(tx *securityTx) error { return fn(tx) })
}

func scanUser(row pgx.Row) (identity.Record, error) {
	var u identity.Record
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Status, &u.PasswordHash, &u.AuthVersion, &u.CreatePage)
	return u, storageError(err)
}

const userColumns = "id::text,username,display_name,role,status,password_hash,auth_version,create_page"

func (t *securityTx) User(ctx context.Context, id string) (identity.Record, error) {
	return scanUser(t.QueryRow(ctx, "SELECT "+userColumns+" FROM wikit.users WHERE id=$1", id))
}
func (t *securityTx) UserByName(ctx context.Context, name string) (identity.Record, error) {
	return scanUser(t.QueryRow(ctx, "SELECT "+userColumns+" FROM wikit.users WHERE username_key=$1", name))
}
func (t *securityTx) InsertUser(ctx context.Context, u identity.Record) error {
	_, err := t.Exec(ctx, "INSERT INTO wikit.users VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", u.ID, u.Username, strings.ToLower(u.Username), u.DisplayName, u.PasswordHash, u.Role, u.Status, u.AuthVersion, u.CreatePage)
	return storageError(err)
}
func (t *securityTx) SaveUser(ctx context.Context, u identity.Record) error {
	_, err := t.Exec(ctx, "UPDATE wikit.users SET display_name=$2,password_hash=$3,role=$4,status=$5,auth_version=$6,create_page=$7 WHERE id=$1", u.ID, u.DisplayName, u.PasswordHash, u.Role, u.Status, u.AuthVersion, u.CreatePage)
	return storageError(err)
}
func (t *securityTx) Token(ctx context.Context, id string) (identity.Token, error) {
	var out identity.Token
	err := t.QueryRow(ctx, "SELECT user_id::text,coalesce(digest,''),generation,created_at FROM wikit.user_tokens WHERE user_id=$1", id).Scan(&out.UserID, &out.Digest, &out.Generation, &out.CreatedAt)
	return out, storageError(err)
}
func (t *securityTx) FindToken(ctx context.Context, digest string) (identity.Token, error) {
	var out identity.Token
	err := t.QueryRow(ctx, "SELECT user_id::text,digest,generation,created_at FROM wikit.user_tokens WHERE digest=$1", digest).Scan(&out.UserID, &out.Digest, &out.Generation, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fault.Unauthenticated
	}
	return out, storageError(err)
}
func (t *securityTx) SaveToken(ctx context.Context, v identity.Token) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.user_tokens VALUES($1,nullif($2,''),$3,$4) ON CONFLICT(user_id) DO UPDATE SET digest=excluded.digest,generation=excluded.generation,created_at=excluded.created_at`, v.UserID, v.Digest, v.Generation, v.CreatedAt)
	return storageError(err)
}
func (t *securityTx) FindSession(ctx context.Context, digest string) (identity.Session, error) {
	var out identity.Session
	err := t.QueryRow(ctx, "SELECT digest,user_id::text,csrf_digest,auth_version,expires_at FROM wikit.sessions WHERE digest=$1", digest).Scan(&out.Digest, &out.UserID, &out.CSRFDigest, &out.AuthVersion, &out.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fault.Unauthenticated
	}
	return out, storageError(err)
}
func (t *securityTx) SaveSession(ctx context.Context, v identity.Session) error {
	_, err := t.Exec(ctx, "INSERT INTO wikit.sessions VALUES($1,$2,$3,$4,$5)", v.Digest, v.UserID, v.CSRFDigest, v.AuthVersion, v.ExpiresAt)
	return storageError(err)
}
func (t *securityTx) DeleteSession(ctx context.Context, digest string) error {
	_, err := t.Exec(ctx, "DELETE FROM wikit.sessions WHERE digest=$1", digest)
	return storageError(err)
}
func (t *securityTx) DeleteSessions(ctx context.Context, id string) error {
	_, err := t.Exec(ctx, "DELETE FROM wikit.sessions WHERE user_id=$1", id)
	return storageError(err)
}
func (t *securityTx) Audit(ctx context.Context, a identity.Actor, action, target, requestID string) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.audit_events(actor_id,method,credential_generation,action,target_id,request_id) VALUES(nullif($1,'')::uuid,$2,$3,$4,nullif($5,''),$6)`, a.ID, a.Method, a.Generation, action, target, requestID)
	return storageError(err)
}
func (s *Store) Allow(ctx context.Context, operation, key string) (result error) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		return fault.BadRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return storageError(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			result = errors.Join(result, errors.New("rate limit rollback failed"))
		}
	}()
	allowed := true
	for i, value := range parts {
		// Hash keys so neither usernames nor network addresses enter counter storage.
		hash := identity.Digest(operation + ":" + string(rune('0'+i)) + ":" + value)
		var attempts int
		err = tx.QueryRow(ctx, `INSERT INTO wikit.auth_limits VALUES($1,date_trunc('minute',now()),1)
		 ON CONFLICT(key) DO UPDATE SET window_start=excluded.window_start,attempts=CASE WHEN auth_limits.window_start=excluded.window_start THEN auth_limits.attempts+1 ELSE 1 END RETURNING attempts`, hash).Scan(&attempts)
		if err != nil {
			return storageError(err)
		}
		maximum := 20
		if i == 1 {
			maximum = 10
		}
		if attempts > maximum {
			allowed = false
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return storageError(err)
	}
	if !allowed {
		return fault.Limited
	}
	return nil
}

func (t *securityTx) GrantedCreate(ctx context.Context, id string) (bool, error) {
	var allowed bool
	err := t.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wikit.groups g JOIN wikit.group_members m ON m.group_id=g.id WHERE m.user_id=$1 AND g.create_page)", id).Scan(&allowed)
	return allowed, storageError(err)
}
