//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	httpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/http"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/health"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/epoch-fresh-2026/Wikit/internal/security"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/blobfs"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

const password = "correct-horse-password"

type suite struct {
	db         database
	accounts   *identity.Service
	access     *access.Service
	secrets    security.Secrets
	admin      identity.Account
	adminToken string
	server     *httptest.Server
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	db := newDatabase(t)
	ctx := context.Background()
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := security.NewPasswords()
	if err != nil {
		t.Fatal(err)
	}
	s := &suite{db: db}
	s.accounts = identity.New(db.store, p, s.secrets, identity.Options{Registration: true, SessionTTL: time.Hour})
	s.access = access.New(db.store, s.secrets)
	s.admin, err = s.accounts.CreateAdmin(ctx, "local_admin", password, "test-init")
	if err != nil {
		t.Fatal(err)
	}
	// Use a session for the locally initialized administrator; no bootstrap Token is exposed.
	login, err := s.accounts.Login(ctx, "local_admin", password, "local-ip", "test-login", true)
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := s.accounts.ChangeSecret(ctx, identity.Credentials{Session: login.Session, CSRF: login.CSRF, OriginOK: true}, password, "", "rotate", "test-rotate")
	if err != nil {
		t.Fatal(err)
	}
	s.adminToken = plain
	api := httpadapter.API{Identity: s.accounts, Access: s.access, Pages: pages.New(db.store, s.secrets, textadapter.Parser{}), Proposals: proposals.New(db.store, s.secrets, textadapter.Parser{}, textadapter.Differ{}), Secrets: s.secrets, PublicURL: "http://localhost:8080", SessionTTL: time.Hour}
	s.server = httptest.NewServer(api.Handler(httpadapter.NewHandler(health.New(db.store, blobfs.Probe{Root: t.TempDir()}))))
	t.Cleanup(s.server.Close)
	return s
}
func (s *suite) register(t *testing.T, name string) (identity.Account, string) {
	t.Helper()
	a, token, err := s.accounts.Register(context.Background(), name, password, "ip-"+name, "register-"+name, true)
	if err != nil {
		t.Fatal(err)
	}
	return a, token
}
func (s *suite) req(t *testing.T, method, path string, body any, c identity.Credentials, key string, expected int) ([]byte, *http.Response) {
	t.Helper()
	var raw []byte
	var err error
	if text, ok := body.(string); ok {
		raw = []byte(text)
	} else if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := http.NewRequest(method, s.server.URL+"/api/v1/"+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if c.OriginOK {
		r.Header.Set("Origin", "http://localhost:8080")
	}
	if c.Bearer != "" {
		r.Header.Set("Authorization", "Bearer "+c.Bearer)
	}
	if c.Session != "" {
		r.AddCookie(&http.Cookie{Name: "wikit_session", Value: c.Session})
	}
	r.Header.Set("X-CSRF-Token", c.CSRF)
	r.Header.Set("Idempotency-Key", key)
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected {
		var e struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		t.Fatalf("%s %s: expected %d got %d (%s)", method, path, expected, response.StatusCode, e.Error.Code)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	return data, response
}
func requestKey(c identity.Credentials, key string) access.Request {
	return access.Request{Credentials: c, ID: "test-request", Key: key, Hash: identity.Digest(key)}
}
func TestAUTH01HTTPAndCSRF(t *testing.T) {
	s := newSuite(t)
	origin := identity.Credentials{OriginOK: true}
	s.req(t, "POST", "auth/register", map[string]string{"username": "new_member", "password": password}, identity.Credentials{}, "", 403)
	data, _ := s.req(t, "POST", "auth/register", map[string]string{"username": "new_member", "password": password}, origin, "", 201)
	var registered struct {
		Account identity.Account `json:"account"`
		Token   string           `json:"token"`
	}
	if err := json.Unmarshal(data, &registered); err != nil {
		t.Fatal(err)
	}
	if registered.Account.Role != "member" || registered.Token == "" {
		t.Fatal("public account became admin or missing Token")
	}
	s.req(t, "POST", "auth/register", map[string]string{"username": "NEW_MEMBER", "password": password}, origin, "", 422)
	s.req(t, "POST", "auth/login", map[string]string{"username": "new_member", "password": "wrong-password"}, origin, "", 401)
	s.req(t, "POST", "auth/login", `{"username":"new_member","password":"correct-horse-password","extra":true}`, origin, "", 400)
	s.req(t, "POST", "auth/login", `{"username":"new_member","password":null}`, origin, "", 400)
	s.req(t, "POST", "auth/login", `{"username":"new_member","password":"correct-horse-password"} {}`, origin, "", 400)
	data, response := s.req(t, "POST", "auth/login", map[string]string{"username": "NEW_MEMBER", "password": password}, origin, "", 200)
	var logged struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(data, &logged); err != nil {
		t.Fatal(err)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" {
		t.Fatal("unsafe cookie")
	}
	c := identity.Credentials{Session: cookies[0].Value, OriginOK: true}
	s.req(t, "POST", "auth/token", map[string]string{"current_password": password}, c, "", 403)
	c.CSRF = logged.CSRF
	s.req(t, "GET", "auth/me", nil, c, "", 200)
	_, otherToken := s.register(t, "other_member")
	mixed := c
	mixed.Bearer = otherToken
	s.req(t, "GET", "auth/me", nil, mixed, "", 400)
	mixed.Bearer = "invalid"
	s.req(t, "GET", "auth/me", nil, mixed, "", 401)
	if strings.Contains(string(data), "password_hash") || strings.Contains(string(data), "digest") {
		t.Fatal("credential storage leaked")
	}
	s.req(t, "POST", "auth/logout", nil, c, "", 204)
	s.req(t, "GET", "auth/me", nil, c, "", 401)
	s.req(t, "POST", "auth/logout", nil, c, "", 204)
}
func TestAUTH02TokenLifecycleAndPassword(t *testing.T) {
	s := newSuite(t)
	u, old := s.register(t, "token_user")
	ctx := context.Background()
	login, err := s.accounts.Login(ctx, u.Username, password, "token-ip", "login", true)
	if err != nil {
		t.Fatal(err)
	}
	c := identity.Credentials{Session: login.Session, CSRF: login.CSRF, OriginOK: true}
	s.req(t, "POST", "auth/token", map[string]string{"current_password": password}, identity.Credentials{Bearer: old}, "", 401)
	data, _ := s.req(t, "POST", "auth/token", map[string]string{"current_password": password}, c, "", 200)
	var rotated struct {
		Token      string `json:"token"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(data, &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.Generation != 2 || rotated.Token == old {
		t.Fatal("not rotated")
	}
	s.req(t, "GET", "auth/me", nil, identity.Credentials{Bearer: old}, "", 401)
	s.req(t, "GET", "auth/me", nil, identity.Credentials{Bearer: rotated.Token}, "", 200)
	s.req(t, "POST", "auth/token/revoke", map[string]string{"current_password": password}, c, "", 204)
	s.req(t, "POST", "auth/token/revoke", map[string]string{"current_password": password}, c, "", 204)
	s.req(t, "GET", "auth/me", nil, identity.Credentials{Bearer: rotated.Token}, "", 401)
	data, _ = s.req(t, "GET", "auth/token", nil, c, "", 200)
	if strings.Contains(string(data), old) || strings.Contains(string(data), rotated.Token) || !strings.Contains(string(data), `"active":false`) {
		t.Fatal("token status leaked or incorrect")
	}
	s.req(t, "POST", "auth/password", map[string]string{"current_password": password, "new_password": "replacement-password"}, c, "", 204)
	s.req(t, "GET", "auth/me", nil, c, "", 401)
	if _, err := s.accounts.Login(ctx, u.Username, password, "new-ip", "login", true); err != fault.Unauthenticated {
		t.Fatal("old password accepted")
	}
	if _, err := s.accounts.Login(ctx, u.Username, "replacement-password", "new-ip", "login", true); err != nil {
		t.Fatal(err)
	}
}
func TestGovernanceAndImmediateDisable(t *testing.T) {
	s := newSuite(t)
	u, token := s.register(t, "governed_user")
	admin := identity.Credentials{Bearer: s.adminToken}
	login, err := s.accounts.Login(context.Background(), u.Username, password, "role-ip", "role-login", true)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"role": "reviewer", "status": "active", "expected_auth_version": 1}
	s.req(t, "PATCH", "admin/accounts/"+u.ID, body, identity.Credentials{Bearer: token}, "account-key-1", 403)
	s.req(t, "PATCH", "admin/accounts/"+u.ID, body, admin, "account-key-1", 200)
	s.req(t, "GET", "auth/me", nil, identity.Credentials{Session: login.Session}, "", 401)
	s.req(t, "PATCH", "admin/accounts/"+u.ID, body, admin, "account-key-1", 200)
	body["role"] = "member"
	s.req(t, "PATCH", "admin/accounts/"+u.ID, body, admin, "account-key-1", 409)
	body["role"] = "reviewer"
	body["status"] = "disabled"
	body["expected_auth_version"] = 2
	login, err = s.accounts.Login(context.Background(), u.Username, password, "role-ip", "role-login", true)
	if err != nil {
		t.Fatal(err)
	}
	s.req(t, "PATCH", "admin/accounts/"+u.ID, body, admin, "account-key-2", 200)
	s.req(t, "GET", "auth/me", nil, identity.Credentials{Bearer: token}, "", 401)
	s.req(t, "GET", "auth/me", nil, identity.Credentials{Session: login.Session}, "", 401)
	s.req(t, "PATCH", "admin/accounts/"+s.admin.ID, map[string]any{"role": "member", "status": "active", "expected_auth_version": 1}, admin, "last-admin-key", 409)
	data, _ := s.req(t, "GET", "admin/accounts?limit=1", nil, admin, "", 200)
	var page struct {
		Next string `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Next == "" {
		t.Fatal("missing cursor")
	}
	s.req(t, "GET", "admin/accounts?limit=1&cursor="+page.Next, nil, admin, "", 200)
	s.req(t, "GET", "admin/accounts?limit=2&cursor="+page.Next, nil, admin, "", 400)
	s.req(t, "GET", "admin/groups?limit=1&cursor="+page.Next, nil, admin, "", 400)
	data, _ = s.req(t, "GET", "admin/audit", nil, admin, "", 200)
	if !strings.Contains(string(data), "account.update") || strings.Contains(string(data), password) || strings.Contains(string(data), s.adminToken) {
		t.Fatal("audit missing or leaks credentials")
	}
}

func TestFirstPublicAccountIsMemberAndSessionStorage(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := security.NewPasswords()
	if err != nil {
		t.Fatal(err)
	}
	svc := identity.New(db.store, p, security.Secrets{}, identity.Options{Registration: true, SessionTTL: time.Hour})
	u, token, err := svc.Register(ctx, "first_public", password, "first-ip", "first-request", true)
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != "member" {
		t.Fatal("first public account became administrator")
	}
	login, err := svc.Login(ctx, u.Username, password, "first-ip", "first-login", true)
	if err != nil {
		t.Fatal(err)
	}
	var hash, tokenDigest, sessionDigest, csrfDigest string
	if err := db.conn.QueryRow(ctx, `SELECT u.password_hash,t.digest,s.digest,s.csrf_digest FROM wikit.users u JOIN wikit.user_tokens t ON t.user_id=u.id JOIN wikit.sessions s ON s.user_id=u.id WHERE u.id=$1`, u.ID).Scan(&hash, &tokenDigest, &sessionDigest, &csrfDigest); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || tokenDigest == token || sessionDigest == login.Session || csrfDigest == login.CSRF || len(tokenDigest) != 64 {
		t.Fatal("plaintext credential storage")
	}
	execSQL(t, db.conn, "UPDATE wikit.sessions SET expires_at=now()-interval '1 second'")
	if _, err := svc.Current(ctx, identity.Credentials{Session: login.Session}); err != fault.Unauthenticated {
		t.Fatal("expired session accepted", err)
	}
}

func TestConcurrentSensitiveRequestsDoNotExhaustPool(t *testing.T) {
	s := newSuite(t)
	u, _ := s.register(t, "parallel_user")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	login, err := s.accounts.Login(ctx, u.Username, password, "parallel-ip", "parallel-login", true)
	if err != nil {
		t.Fatal(err)
	}
	c := identity.Credentials{Session: login.Session, CSRF: login.CSRF, OriginOK: true}
	start := make(chan struct{})
	results := make(chan error, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := s.accounts.ChangeSecret(ctx, c, password, "", "revoke", "parallel-request")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && err != fault.Limited {
			t.Fatal("nested pool acquisition or unexpected error", err)
		}
	}
}

func TestUUIDCaseNormalization(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	u, _ := s.register(t, "mixed_case_user")
	admin := identity.Credentials{Bearer: s.adminToken}
	if _, err := s.access.UpdateAccount(ctx, requestKey(admin, "uppercase-account"), strings.ToUpper(u.ID), "reviewer", "active", 1); err != nil {
		t.Fatal("valid uppercase UUID rejected", err)
	}
	data, err := s.access.SaveGroup(ctx, requestKey(admin, "case-group-create"), "", "Case group", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var group access.Group
	if err := json.Unmarshal(data, &group); err != nil {
		t.Fatal(err)
	}
	if _, err := s.access.SaveGroup(ctx, requestKey(admin, "case-group-update"), strings.ToUpper(group.ID), group.Name, []string{strings.ToUpper(u.ID)}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.access.SaveGroup(ctx, requestKey(admin, "case-duplicate"), group.ID, group.Name, []string{u.ID, strings.ToUpper(u.ID)}, 2); err != fault.Invalid {
		t.Fatal("case aliases bypassed duplicate detection", err)
	}
	page := s.id(t)
	s.seedPolicy(t, access.Policy{PageID: page, Visibility: "restricted", Status: "published", Version: 1, Grants: []access.Grant{{SubjectType: "user", SubjectID: u.ID, Capabilities: []string{"read"}}}})
	if _, err := s.access.UpdatePolicy(ctx, requestKey(admin, "case-policy-update"), strings.ToUpper(page), access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: strings.ToUpper(u.ID), Capabilities: []string{"read"}}}}, 1); err != nil {
		t.Fatal(err)
	}
}
func TestRateLimitAndRegistrationOff(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if err := s.db.store.Allow(ctx, "probe", "ip\x00name"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.store.Allow(ctx, "probe", "ip\x00name"); err != fault.Limited {
		t.Fatal("limit not persisted")
	}
	for i := 0; i < 11; i++ {
		_, err := s.accounts.Login(ctx, "absent_user", password, "failed-ip", "failed", true)
		if i < 10 && err != fault.Unauthenticated {
			t.Fatal(err)
		}
		if i == 10 && err != fault.Limited {
			t.Fatal("failed login counters rolled back", err)
		}
	}
	p, err := security.NewPasswords()
	if err != nil {
		t.Fatal(err)
	}
	closed := identity.New(s.db.store, p, s.secrets, identity.Options{Registration: false, SessionTTL: time.Hour})
	if _, _, err := closed.Register(ctx, "closed_user", password, "ip", "request", true); err != fault.Forbidden {
		t.Fatal("registration enabled")
	}
}
