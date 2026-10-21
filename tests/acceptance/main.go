// Command acceptance is an isolated CI driver, excluded from production images.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/security"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/blobfs"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
)

type state struct {
	AdminToken, MemberToken, Session, CSRF string
	Document, Private, Media               pages.Created
	Revision, MediaRevision                string
}

var ctx = context.Background()
var secrets = security.Secrets{}
var base = "http://wikit:8080"
var client = &http.Client{Timeout: 20 * time.Second}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	// This program is never a supported production administration interface.
	if os.Getenv("WIKIT_ACCEPTANCE") != "isolated" || len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "isolated acceptance environment required")
		os.Exit(1)
	}
	defer func() {
		if failure := recover(); failure != nil {
			if message, ok := failure.(string); ok {
				fmt.Fprintln(os.Stderr, message)
			}
			fmt.Fprintf(os.Stderr, "acceptance %s failed (dependency details suppressed to protect credentials)\n", os.Args[1])
			os.Exit(1)
		}
	}()
	switch os.Args[1] {
	case "init":
		initialize()
	case "smoke":
		smoke()
	case "verify", "verify-api":
		verify(false)
	case "restored":
		verify(true)
	case "audit":
		audit()
	case "seed":
		seed()
	case "bench":
		benchmark()
	default:
		panic("unknown command")
	}
}
func save(s state) {
	b, err := json.Marshal(s)
	must(err)
	must(os.WriteFile("/state/state.json", b, 0600))
}
func load() state {
	var s state
	b, err := os.ReadFile("/state/state.json")
	must(err)
	must(json.Unmarshal(b, &s))
	return s
}
func request(method, path, token string, body any, status int) []byte {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		must(err)
		payload = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, base+path, payload)
	must(err)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if method != "GET" {
		id, err := secrets.ID()
		must(err)
		r.Header.Set("Idempotency-Key", id)
	}
	resp, err := client.Do(r)
	must(err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	must(err)
	if resp.StatusCode != status {
		panic(fmt.Sprintf("%s %s status %d expected %d", method, path, resp.StatusCode, status))
	}
	return b
}
func decode[T any](b []byte) T { var out T; must(json.Unmarshal(b, &out)); return out }
func login(name string) (token, session, csrf string) {
	jar, err := cookiejar.New(nil)
	must(err)
	c := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	body, err := json.Marshal(map[string]string{"username": name, "password": os.Getenv("ACCEPTANCE_PASSWORD")})
	must(err)
	r, err := http.NewRequest("POST", base+"/api/v1/auth/login", bytes.NewReader(body))
	must(err)
	r.Header.Set("Origin", os.Getenv("WIKIT_PUBLIC_URL"))
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(r)
	must(err)
	b, err := io.ReadAll(resp.Body)
	must(err)
	must(resp.Body.Close())
	if resp.StatusCode != 200 {
		panic("login failed")
	}
	csrf = decode[struct {
		CSRF string `json:"csrf_token"`
	}](b).CSRF
	for _, cookie := range jar.Cookies(r.URL) {
		if cookie.Name == "wikit_session" {
			session = cookie.Value
		}
	}
	body, err = json.Marshal(map[string]string{"current_password": os.Getenv("ACCEPTANCE_PASSWORD")})
	must(err)
	r, err = http.NewRequest("POST", base+"/api/v1/auth/token", bytes.NewReader(body))
	must(err)
	r.Header.Set("Origin", os.Getenv("WIKIT_PUBLIC_URL"))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	resp, err = c.Do(r)
	must(err)
	b, err = io.ReadAll(resp.Body)
	must(err)
	must(resp.Body.Close())
	if resp.StatusCode != 200 {
		panic("rotation failed")
	}
	token = decode[struct {
		Token string `json:"token"`
	}](b).Token
	return
}
func initialize() {
	db, err := postgres.Open(ctx, os.Getenv("WIKIT_DATABASE_URL"))
	must(err)
	defer db.Close()
	p, err := security.NewPasswords()
	must(err)
	id, err := secrets.ID()
	must(err)
	_, err = identity.New(db, p, secrets, identity.Options{Registration: true, SessionTTL: time.Hour}).CreateAdmin(ctx, "acceptance_admin", os.Getenv("ACCEPTANCE_PASSWORD"), id)
	must(err)
	fmt.Println("isolated administrator initialized")
}
func publish(token, admin, visibility string, snapshot map[string]any) (pages.Created, string) {
	p := decode[pages.Created](request("POST", "/api/v1/pages", token, map[string]any{"visibility": visibility, "snapshot": snapshot}, 201))
	expected := map[string]any{"expected_head_revision_id": nil, "expected_proposal_revision_id": p.Proposal.CurrentRevision.ID}
	review := map[string]any{"expected_head_revision_id": nil, "expected_proposal_revision_id": p.Proposal.CurrentRevision.ID, "expected_review_round": p.Proposal.ReviewRound, "decision": "approve", "body": "isolated acceptance review"}
	request("POST", "/api/v1/proposals/"+p.Proposal.ID+"/reviews", admin, review, 201)
	revision := decode[content.Published](request("POST", "/api/v1/proposals/"+p.Proposal.ID+"/merge", admin, expected, 200))
	return p, revision.ID
}

var png = []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 31, 21, 196, 137, 0, 0, 0, 11, 73, 68, 65, 84, 120, 156, 99, 96, 0, 2, 0, 0, 5, 0, 1, 165, 246, 69, 64, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}

func smoke() {
	s := state{}
	s.AdminToken, s.Session, s.CSRF = login("acceptance_admin")
	reg := decode[struct {
		Token string `json:"token"`
	}](requestOriginRegister())
	s.MemberToken = reg.Token
	s.Document, s.Revision = publish(s.MemberToken, s.AdminToken, "public", map[string]any{"type": "document", "title": "恢复验收 recovery marker", "tags": []string{"acceptance"}, "markdown": "历史正文 immutable history"})
	s.Private, _ = publish(s.MemberToken, s.AdminToken, "restricted", map[string]any{"type": "document", "title": "restricted secret marker", "tags": []string{}, "markdown": "private body"})
	u := decode[struct {
		Path string `json:"upload_path"`
	}](request("POST", "/api/v1/uploads", s.MemberToken, map[string]any{"filename": "acceptance.png", "size": len(png), "mime": "image/png"}, 201))
	r, err := http.NewRequest("PUT", base+u.Path, bytes.NewReader(png))
	must(err)
	r.Header.Set("Content-Type", "image/png")
	r.Header.Set("Authorization", "Bearer "+s.MemberToken)
	resp, err := client.Do(r)
	must(err)
	b, err := io.ReadAll(resp.Body)
	must(err)
	must(resp.Body.Close())
	if resp.StatusCode != 200 {
		panic("upload failed")
	}
	blob := decode[struct {
		ID string `json:"id"`
	}](b)
	s.Media, s.MediaRevision = publish(s.MemberToken, s.AdminToken, "public", map[string]any{"type": "media", "title": "恢复原件", "tags": []string{}, "description": "original media", "blob_id": blob.ID})
	save(s)
	verify(false)
	fmt.Println("real REST create/review/merge/history/media and authentication smoke passed")
}
func requestOriginRegister() []byte {
	body, err := json.Marshal(map[string]string{"username": "acceptance_member", "password": os.Getenv("ACCEPTANCE_PASSWORD")})
	must(err)
	r, err := http.NewRequest("POST", base+"/api/v1/auth/register", bytes.NewReader(body))
	must(err)
	r.Header.Set("Origin", os.Getenv("WIKIT_PUBLIC_URL"))
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	must(err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	must(err)
	if resp.StatusCode != 201 {
		panic("registration failed")
	}
	return b
}
func verify(restored bool) {
	s := load()
	if restored {
		request("GET", "/api/v1/auth/me", s.AdminToken, nil, 401)
		request("GET", "/api/v1/auth/me", s.MemberToken, nil, 401)
		r, err := http.NewRequest("GET", base+"/api/v1/auth/me", nil)
		must(err)
		r.AddCookie(&http.Cookie{Name: "wikit_session", Value: s.Session})
		resp, err := client.Do(r)
		must(err)
		must(resp.Body.Close())
		if resp.StatusCode != 401 {
			panic("restored session remains valid")
		}
		s.AdminToken, s.Session, s.CSRF = login("acceptance_admin")
		s.MemberToken, _, _ = login("acceptance_member")
		save(s)
		fmt.Println("restored credentials revoked; administrators and members can log in again")
		anonymous := request("GET", "/api/v1/search?q=restricted%20secret%20marker", "", nil, 200)
		if bytes.Contains(anonymous, []byte(s.Private.Page.ID)) {
			panic("restricted search leaked after restore")
		}
		member := request("GET", "/api/v1/search?q=restricted%20secret%20marker", s.MemberToken, nil, 200)
		if !bytes.Contains(member, []byte(s.Private.Page.ID)) {
			panic("authorized search not rebuilt")
		}
		fmt.Println("rebuilt search excludes restricted content for anonymous users and returns it to its authorized author")
	}
	page := request("GET", "/api/v1/pages/"+s.Document.Page.ID, "", nil, 200)
	if !bytes.Contains(page, []byte("历史正文")) {
		panic("published body lost")
	}
	request("GET", "/api/v1/pages/"+s.Document.Page.ID+"/revisions/"+s.Revision, "", nil, 200)
	history := request("GET", "/api/v1/pages/"+s.Document.Page.ID+"/revisions", "", nil, 200)
	if !bytes.Contains(history, []byte(s.Revision)) {
		panic("publication history lost")
	}
	proposal := request("GET", "/api/v1/proposals/"+s.Document.Proposal.ID, s.MemberToken, nil, 200)
	if !bytes.Contains(proposal, []byte(`"state":"merged"`)) || !bytes.Contains(proposal, []byte(s.Document.Proposal.CurrentRevision.ID)) {
		panic("proposal state or candidate history lost")
	}
	request("GET", "/api/v1/pages/"+s.Private.Page.ID, "", nil, 404)
	request("GET", "/api/v1/pages/"+s.Private.Page.ID, s.MemberToken, nil, 200)
	b := request("GET", "/api/v1/pages/"+s.Media.Page.ID+"/revisions/"+s.MediaRevision+"/content", "", nil, 200)
	if !bytes.Equal(b, png) {
		panic("media original mismatch")
	}
	if os.Args[1] != "verify-api" {
		for _, p := range []string{"/", "/pages/" + s.Document.Page.ID, "/login"} {
			b := request("GET", p, "", nil, 200)
			if !bytes.Contains(b, []byte("/assets/")) {
				panic("frontend missing")
			}
			for _, match := range regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`).FindAllSubmatch(b, -1) {
				asset := request("GET", string(match[1]), "", nil, 200)
				if len(asset) < 100 || bytes.HasPrefix(asset, []byte("<!doctype html>")) {
					panic("frontend asset missing")
				}
			}
		}
	}
	b = request("GET", "/api/v1/unknown", "", nil, 404)
	if bytes.Contains(b, []byte("<html")) {
		panic("API became SPA")
	}
	fmt.Println("public/restricted reads, fixed history, original bytes and embedded SPA passed")
}
func audit() {
	conn, err := pgx.Connect(ctx, os.Getenv("WIKIT_DATABASE_URL"))
	must(err)
	defer func() { must(conn.Close(ctx)) }()
	files, err := blobfs.New(os.Getenv("WIKIT_MEDIA_ROOT"))
	must(err)
	defer func() { must(files.Close()) }()
	rows, err := conn.Query(ctx, `SELECT DISTINCT b.storage_key,b.digest,b.size FROM wikit.blobs b WHERE b.id IN (SELECT blob_id FROM wikit.revisions WHERE blob_id IS NOT NULL UNION SELECT blob_id FROM wikit.proposal_revisions WHERE blob_id IS NOT NULL)`)
	must(err)
	defer rows.Close()
	n := 0
	for rows.Next() {
		var key, digest string
		var size int64
		must(rows.Scan(&key, &digest, &size))
		f, err := files.Open(ctx, key)
		must(err)
		h := sha256.New()
		count, err := io.Copy(h, f)
		must(err)
		must(f.Close())
		if count != size || hex.EncodeToString(h.Sum(nil)) != digest {
			panic("reachable blob corrupt")
		}
		n++
	}
	must(rows.Err())
	if n == 0 {
		panic("no reachable media audited")
	}
	fmt.Printf("reachable originals SHA-256 and size verified: %d\n", n)
}
func seed() {
	conn, err := pgx.Connect(ctx, os.Getenv("WIKIT_DATABASE_URL"))
	must(err)
	defer func() { must(conn.Close(ctx)) }()
	tx, err := conn.Begin(ctx)
	must(err)
	defer func() {
		err := tx.Rollback(ctx)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			panic(err)
		}
	}()
	// Offline fixture only. Every FK, immutability/seal and head constraint stays enabled.
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE fixture AS SELECT n,gen_random_uuid() p,gen_random_uuid() pr,gen_random_uuid() c,gen_random_uuid() r FROM generate_series(1,100000) n;
INSERT INTO wikit.page_policies SELECT p,CASE WHEN n%5=0 THEN 'restricted' ELSE 'public' END,CASE WHEN n%10=0 THEN 'hidden' ELSE 'published' END,1 FROM fixture;
INSERT INTO wikit.pages SELECT p,'document',NULL FROM fixture;
INSERT INTO wikit.proposals SELECT pr,p,(SELECT id FROM wikit.users WHERE username='acceptance_member'),'merged',1,1,c FROM fixture;
INSERT INTO wikit.proposal_revisions(id,proposal_id,page_id,type,snapshot,created_at,sealed) SELECT c,pr,p,'document',jsonb_build_object('type','document','title','benchmark knowledge 知识 '||n,'tags','[]'::jsonb,'markdown',repeat('knowledge 知识 recovery 基准 ',40)),now(),true FROM fixture;
INSERT INTO wikit.revisions(id,page_id,type,source_proposal_id,snapshot,author_id,created_at,sealed) SELECT r,p,'document',pr,jsonb_build_object('type','document','title','benchmark knowledge 知识 '||n,'tags','[]'::jsonb,'markdown',repeat('knowledge 知识 recovery 基准 ',40)),(SELECT id FROM wikit.users WHERE username='acceptance_member'),now(),true FROM fixture;
UPDATE wikit.pages p SET head_revision_id=f.r FROM fixture f WHERE p.id=f.p;`)
	must(err)
	must(tx.Commit(ctx))
	fmt.Println("100000 offline fixture Pages seeded (80% public, 10% restricted, 10% hidden; ~1KiB bilingual bodies)")
}

type metric struct {
	Phase                         string `json:"phase"`
	Requests, Concurrency, Errors int
	P50                           float64 `json:"p50_ms"`
	P95                           float64 `json:"p95_ms"`
	P99                           float64 `json:"p99_ms"`
	RPS                           float64
	Seconds                       float64
	ErrorRate                     float64 `json:"error_rate"`
}

func benchmark() {
	// Search must already have been rebuilt from the 100000-page database.
	conn, err := pgx.Connect(ctx, os.Getenv("WIKIT_DATABASE_URL"))
	must(err)
	defer func() { must(conn.Close(ctx)) }()
	var pages int
	must(conn.QueryRow(ctx, "SELECT count(*) FROM wikit.pages").Scan(&pages))
	if pages < 100000 {
		panic("benchmark dataset too small")
	}
	public := map[string]bool{}
	rows, err := conn.Query(ctx, "SELECT page_id::text FROM wikit.page_policies WHERE visibility='public' AND status='published'")
	must(err)
	for rows.Next() {
		var id string
		must(rows.Scan(&id))
		public[id] = true
	}
	must(rows.Err())
	rows.Close()
	for _, phase := range []string{"first-pass", "warm"} {
		start := time.Now()
		var wg sync.WaitGroup
		var mu sync.Mutex
		latencies := []float64{}
		failures := 0
		for c := 0; c < 100; c++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 20; i++ {
					begin := time.Now()
					bad := false
					func() {
						defer func() {
							if recover() != nil {
								bad = true
							}
						}()
						b := request("GET", "/api/v1/search?q=knowledge&limit=20", "", nil, 200)
						var result struct {
							Items []struct {
								ID string `json:"page_id"`
							}
						}
						must(json.Unmarshal(b, &result))
						if len(result.Items) == 0 {
							panic("empty search")
						}
						for _, item := range result.Items {
							if !public[item.ID] {
								panic("unauthorized search hit")
							}
						}
					}()
					mu.Lock()
					latencies = append(latencies, float64(time.Since(begin).Microseconds())/1000)
					if bad {
						failures++
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		sort.Float64s(latencies)
		seconds := time.Since(start).Seconds()
		m := metric{Phase: phase, Requests: len(latencies), Concurrency: 100, Errors: failures, P50: latencies[len(latencies)*50/100], P95: latencies[len(latencies)*95/100], P99: latencies[len(latencies)*99/100], RPS: float64(len(latencies)) / seconds, Seconds: seconds, ErrorRate: float64(failures) / float64(len(latencies))}
		b, err := json.Marshal(m)
		must(err)
		fmt.Println(string(b))
		if failures > 0 {
			panic("benchmark errors or permission leaks")
		}
	}
}
