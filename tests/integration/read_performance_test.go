//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/http"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

type batchKey struct{}
type measuredIndex struct{ search.Index }

func (i measuredIndex) Search(ctx context.Context, name, query string, f search.Filter, offset, limit int) (search.Candidates, error) {
	if count, ok := ctx.Value(batchKey{}).(*atomic.Int64); ok {
		count.Add(1)
	}
	return i.Index.Search(ctx, name, query, f, offset, limit)
}

// Opt-in scale measurement, separate from mandatory functional integration tests.
// Every invocation owns its database and a unique search index through newSuite.
func TestReadPerformance(t *testing.T) {
	raw := os.Getenv("WIKIT_PERF_PAGES")
	if raw == "" {
		t.Skip("opt-in scale measurement: set WIKIT_PERF_PAGES")
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 1000 || count > 100000 {
		t.Fatal("WIKIT_PERF_PAGES must be 1000..100000")
	}
	s := newSearchSuite(t)
	ctx := context.Background()
	tx, err := s.db.conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Offline synthetic fixtures keep all constraints/triggers enabled.
	sql := fmt.Sprintf(`CREATE TEMP TABLE fixture AS SELECT n,nextval('wikit.page_ids') p,gen_random_uuid() r FROM generate_series(1,%d) n;
 INSERT INTO wikit.page_policies SELECT p,CASE WHEN n%%5=0 THEN 'restricted' ELSE 'public' END,CASE WHEN n%%10=0 THEN 'hidden' ELSE 'published' END,1 FROM fixture;
 INSERT INTO wikit.pages SELECT p,'document',NULL FROM fixture;
 INSERT INTO wikit.revisions(id,page_id,type,source_kind,snapshot,author_id,publisher_id,created_at,sealed)
 SELECT r,p,'document','create',jsonb_build_object('type','document','title','benchmark knowledge 知识 '||n,'tags','[]'::jsonb,'markdown',repeat('knowledge 知识 recovery 基准 ',40)),
 (SELECT id FROM wikit.users WHERE username='local_admin'),(SELECT id FROM wikit.users WHERE username='local_admin'),now(),true FROM fixture;
 UPDATE wikit.pages p SET head_revision_id=f.r FROM fixture f WHERE p.id=f.p;`, count)
	if _, err := tx.Exec(ctx, sql); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := s.worker.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("environment os=%s arch=%s cpus=%d go=%s pages=%d rebuild_seconds=%.3f", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version(), count, time.Since(started).Seconds())
	// Independent database-derived allowlist, never infer ACL from search results.
	allowed := map[string]bool{}
	rows, err := s.db.conn.Query(ctx, "SELECT page_id::text FROM wikit.page_policies WHERE visibility='public' AND status='published'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		allowed[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	api := httpadapter.API{Search: search.New(s.db.store, measuredIndex{s.index}, textadapter.Parser{}, s.secrets), Pages: pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files), Secrets: s.secrets}
	handler := api.Handler(http.NotFoundHandler())
	const requests, concurrency = 200, 100
	type sample struct{ SQL, Pool, Wait, Held, Batches float64 }
	metrics := make(chan sample, requests)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, m := postgres.Measure(r.Context())
		batches := &atomic.Int64{}
		handler.ServeHTTP(w, r.WithContext(context.WithValue(ctx, batchKey{}, batches)))
		metrics <- sample{float64(m.Queries.Load()), float64(m.PoolNanos.Load()) / 1e6, float64(m.LockWaitNanos.Load()) / 1e6, float64(m.LockHeldNanos.Load()) / 1e6, float64(batches.Load())}
	}))
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Minute}
	for _, path := range []string{"search?q=knowledge&limit=20", "pages?limit=20"} {
		for _, phase := range []string{"first-pass", "warm"} {
			durations := make([]float64, requests)
			var failures atomic.Int64
			var next atomic.Int64
			var wg sync.WaitGroup
			start := time.Now()
			for n := 0; n < concurrency; n++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						pos := int(next.Add(1)) - 1
						if pos >= requests {
							return
						}
						begin := time.Now()
						res, err := client.Get(server.URL + "/api/v1/" + path)
						if err != nil {
							failures.Add(1)
							durations[pos] = float64(time.Since(begin)) / 1e6
							continue
						}
						data, readErr := io.ReadAll(res.Body)
						closeErr := res.Body.Close()
						var out struct {
							Items []struct {
								PageID string `json:"page_id"`
								Page   struct {
									ID string `json:"id"`
								} `json:"page"`
							} `json:"items"`
						}
						decodeErr := json.Unmarshal(data, &out)
						bad := readErr != nil || closeErr != nil || res.StatusCode != 200 || decodeErr != nil || len(out.Items) != 20
						for _, item := range out.Items {
							id := item.PageID
							if id == "" {
								id = item.Page.ID
							}
							if !allowed[id] {
								bad = true
							}
						}
						if bad {
							failures.Add(1)
						}
						durations[pos] = float64(time.Since(begin)) / 1e6
					}
				}()
			}
			wg.Wait()
			elapsed := time.Since(start).Seconds()
			total := sample{}
			for n := 0; n < requests; n++ {
				select {
				case m := <-metrics:
					total.SQL += m.SQL
					total.Pool += m.Pool
					total.Wait += m.Wait
					total.Held += m.Held
					total.Batches += m.Batches
				case <-time.After(5 * time.Second):
					t.Fatal("missing measurement (request failed before handler)")
				}
			}
			sort.Float64s(durations)
			result := map[string]any{"path": path, "phase": phase, "requests": requests, "concurrency": concurrency, "errors": failures.Load(), "p50_ms": durations[99], "p95_ms": durations[189], "p99_ms": durations[197], "rps": requests / elapsed, "sql_per_request": total.SQL / requests, "pool_ms_per_request": total.Pool / requests, "lock_wait_ms_per_request": total.Wait / requests, "lock_held_ms_per_request": total.Held / requests, "batches_per_request": total.Batches / requests}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(encoded))
			if failures.Load() != 0 {
				t.Fatal("load errors or authorization mismatch")
			}
		}
	}
}
