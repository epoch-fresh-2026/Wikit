package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/jackc/pgx/v5"
)

const SearchWorkerLock int64 = 8734281004

func (s *Store) WithinSearch(ctx context.Context, fn func(search.Tx) error) error {
	return s.withinSecurity(ctx, func(t *securityTx) error { return fn(t) })
}
func (t *securityTx) ActiveIndex(ctx context.Context) (string, error) {
	var name string
	err := t.QueryRow(ctx, `SELECT active_index FROM wikit.search_state WHERE singleton`).Scan(&name)
	return name, storageError(err)
}
func (t *securityTx) SearchSource(ctx context.Context, id string) (search.Source, error) {
	var out search.Source
	var raw []byte
	err := t.QueryRow(ctx, `SELECT p.id::text,p.type,a.visibility,a.status,p.head_revision_id::text,a.policy_version,r.snapshot FROM wikit.pages p JOIN wikit.page_policies a ON a.page_id=p.id LEFT JOIN wikit.revisions r ON r.id=p.head_revision_id AND r.page_id=p.id AND r.sealed WHERE p.id=$1`, id).Scan(&out.Page.ID, &out.Page.Type, &out.Page.Visibility, &out.Page.Status, &out.Page.HeadRevisionID, &out.Page.PolicyVersion, &raw)
	if err != nil {
		return out, storageError(err)
	}
	if raw != nil {
		if err := json.Unmarshal(raw, &out.Snapshot); err != nil {
			return out, storageError(err)
		}
	}
	out.Policy, err = t.Policy(ctx, id)
	return out, err
}
func (t *securityTx) Repair(ctx context.Context, id string) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.outbox_events(id,page_id,kind,created_at) VALUES(gen_random_uuid(),$1,'page.repair',now()) ON CONFLICT(page_id) WHERE kind='page.repair' AND completed_at IS NULL DO NOTHING`, id)
	return storageError(err)
}

// A dedicated pooled session owns this lock until all remote work/checkpoints end.
// Business writes use a different lock and remain available during search faults.
func (s *Store) WithSearchWorker(ctx context.Context, fn func(search.Queue) error) (result error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return storageError(err)
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, SearchWorkerLock).Scan(&locked); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return errors.Join(storageError(err), conn.Conn().Close(cleanup))
	}
	if !locked {
		return fault.State
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRow(cleanup, `SELECT pg_advisory_unlock($1)`, SearchWorkerLock).Scan(&unlocked); err != nil || !unlocked {
			closeErr := conn.Conn().Close(cleanup)
			result = errors.Join(result, errors.New("search worker unlock failed"), closeErr)
		}
	}()
	return fn(&searchQueue{conn.Conn()})
}

type searchQueue struct{ *pgx.Conn }

// Projection reads use one short repeatable snapshot for Page/head and grants.
func (q *searchQueue) SearchSource(ctx context.Context, id string) (out search.Source, result error) {
	tx, err := q.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, storageError(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			result = errors.Join(result, errors.New("search snapshot rollback failed"))
		}
	}()
	out, err = (&securityTx{tx}).SearchSource(ctx, id)
	if err != nil {
		return out, err
	}
	return out, storageError(tx.Commit(ctx))
}
func (q *searchQueue) ActiveIndex(ctx context.Context) (string, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT active_index FROM wikit.search_state WHERE singleton`).Scan(&name)
	return name, storageError(err)
}
func (q *searchQueue) NextEvent(ctx context.Context) (*search.Event, error) {
	var v search.Event
	err := q.QueryRow(ctx, `SELECT e.sequence,e.page_id::text,e.task_uid,coalesce(e.target_index,''),e.attempts FROM wikit.outbox_events e WHERE e.completed_at IS NULL AND e.next_attempt<=now() AND NOT EXISTS(SELECT 1 FROM wikit.outbox_events prior WHERE prior.page_id=e.page_id AND prior.sequence<e.sequence AND prior.completed_at IS NULL) ORDER BY e.sequence LIMIT 1`).Scan(&v.Sequence, &v.Page, &v.Task, &v.Target, &v.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, storageError(err)
}
func (q *searchQueue) SaveTask(ctx context.Context, seq int64, target string, uid int64) error {
	_, err := q.Exec(ctx, `UPDATE wikit.outbox_events SET task_uid=$2,target_index=$3 WHERE sequence=$1 AND completed_at IS NULL`, seq, uid, target)
	return storageError(err)
}
func (q *searchQueue) FailEvent(ctx context.Context, seq int64, code string, clear bool, delay time.Duration) error {
	_, err := q.Exec(ctx, `UPDATE wikit.outbox_events SET attempts=attempts+1,last_error=$2,next_attempt=now()+$3*interval '1 second',task_uid=CASE WHEN $4 THEN NULL ELSE task_uid END WHERE sequence=$1 AND completed_at IS NULL`, seq, code, delay.Seconds(), clear)
	return storageError(err)
}
func (q *searchQueue) CompleteEvent(ctx context.Context, seq int64) error {
	_, err := q.Exec(ctx, `UPDATE wikit.outbox_events SET completed_at=now(),last_error=NULL WHERE sequence=$1`, seq)
	return storageError(err)
}
func (q *searchQueue) Export(ctx context.Context, after int64, all bool) (out search.Export, result error) {
	tx, err := q.Begin(ctx)
	if err != nil {
		return out, storageError(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			result = errors.Join(result, errors.New("search export rollback failed"))
		}
	}()
	// Serialize only the short watermark read with protected publishers. A sequence
	// allocated by an uncommitted writer cannot fall below this committed watermark.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, SecurityLock); err != nil {
		return out, storageError(err)
	}
	err = tx.QueryRow(ctx, `SELECT coalesce((SELECT max(sequence) FROM wikit.outbox_events),0),CASE WHEN $2 THEN ARRAY(SELECT id::text FROM wikit.pages ORDER BY id) ELSE ARRAY(SELECT DISTINCT page_id::text FROM wikit.outbox_events WHERE sequence>$1 ORDER BY page_id::text) END`, after, all).Scan(&out.Watermark, &out.IDs)
	if err != nil {
		return out, storageError(err)
	}
	return out, storageError(tx.Commit(ctx))
}
func (q *searchQueue) SwitchIndex(ctx context.Context, name string) error {
	_, err := q.Exec(ctx, `UPDATE wikit.search_state SET active_index=$1 WHERE singleton`, name)
	return storageError(err)
}
func (q *searchQueue) RetryEvents(ctx context.Context) error {
	_, err := q.Exec(ctx, `UPDATE wikit.outbox_events SET next_attempt=now() WHERE completed_at IS NULL AND attempts>0`)
	return storageError(err)
}
func (q *searchQueue) SearchStats(ctx context.Context) (search.Stats, error) {
	var out search.Stats
	err := q.QueryRow(ctx, `SELECT count(*) FILTER(WHERE completed_at IS NULL),coalesce(extract(epoch FROM now()-min(created_at) FILTER(WHERE completed_at IS NULL))::bigint,0),coalesce(sum(attempts),0) FROM wikit.outbox_events`).Scan(&out.Pending, &out.OldestSeconds, &out.Failures)
	return out, storageError(err)
}
func (s *Store) SearchStats(ctx context.Context) (search.Stats, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return search.Stats{}, storageError(err)
	}
	defer conn.Release()
	return (&searchQueue{conn.Conn()}).SearchStats(ctx)
}
