//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func TestAUTHSessionRecovery(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	login, err := s.accounts.Login(ctx, s.admin.Username, password, "recovery-ip", "restore-login", true)
	if err != nil {
		t.Fatal(err)
	}
	c := identity.Credentials{Session: login.Session, OriginOK: true}
	// Model an existing pre-upgrade session, whose random nonce must keep working.
	legacy := "legacy-independent-random-csrf"
	if _, err := s.db.conn.Exec(ctx, `UPDATE wikit.sessions SET csrf_digest=$1 WHERE digest=$2`, identity.Digest(legacy), identity.Digest(c.Session)); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := s.db.conn.QueryRow(ctx, `SELECT row_to_json(s)::text FROM wikit.sessions s WHERE digest=$1`, identity.Digest(c.Session)).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var csrf string
	for i := 0; i < 3; i++ {
		data, response := s.req(t, "POST", "auth/session", "{}", c, "", 200)
		var state struct {
			Account identity.Account `json:"account"`
			CSRF    string           `json:"csrf_token"`
		}
		if err := json.Unmarshal(data, &state); err != nil {
			t.Fatal(err)
		}
		if state.Account.ID != s.admin.ID || !state.Account.CanCreatePage || len(state.CSRF) != 64 {
			t.Fatal("missing current account or CSRF")
		}
		if i > 0 && csrf != state.CSRF {
			t.Fatal("recovery invalidates another tab")
		}
		csrf = state.CSRF
		if len(response.Cookies()) != 0 || strings.Contains(string(data), c.Session) {
			t.Fatal("recovery exposed or rotated session")
		}
	}
	var after string
	if err := s.db.conn.QueryRow(ctx, `SELECT row_to_json(s)::text FROM wikit.sessions s WHERE digest=$1`, identity.Digest(c.Session)).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("recovery mutated or extended the session")
	}
	// Both legacy tabs and newly recovered tabs pass CSRF before password checks.
	for _, nonce := range []string{legacy, csrf} {
		valid := c
		valid.CSRF = nonce
		s.req(t, "POST", "auth/token", `{"current_password":"incorrect-password"}`, valid, "", 401)
		s.req(t, "POST", "auth/session", "{}", c, "", 200)
	}
	for _, nonce := range []string{"", "wrong"} {
		invalid := c
		invalid.CSRF = nonce
		s.req(t, "POST", "auth/token", `{"current_password":"incorrect-password"}`, invalid, "", 403)
	}
	for _, body := range []string{"null", "", "[]", `{"extra":true}`, "{} {}"} {
		s.req(t, "POST", "auth/session", body, c, "", 400)
	}
	s.req(t, "POST", "auth/session", "{}", identity.Credentials{Session: c.Session}, "", 403)
	for _, origin := range []string{"http://evil.invalid", "null", "http://localhost:8080.evil.invalid"} {
		request, err := http.NewRequest("POST", s.server.URL+"/api/v1/auth/session", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{Name: "wikit_session", Value: c.Session})
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 403 || response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("cross-origin recovery allowed")
		}
	}

	s.req(t, "POST", "auth/session", "{}", identity.Credentials{OriginOK: true}, "", 401)
	s.req(t, "POST", "auth/session", "{}", identity.Credentials{OriginOK: true, Bearer: s.adminToken}, "", 400)
	mixed := c
	mixed.Bearer = s.adminToken
	s.req(t, "POST", "auth/session", "{}", mixed, "", 400)
	invalid := c
	invalid.Session = "unknown-session"
	s.req(t, "POST", "auth/session", "{}", invalid, "", 401)
	// Expiration, disabled accounts and auth-version changes are never recovered.
	for _, change := range []string{
		`UPDATE wikit.sessions SET expires_at=now()-interval '1 second' WHERE digest=$1`,
		`UPDATE wikit.users SET status='disabled' WHERE id=(SELECT user_id FROM wikit.sessions WHERE digest=$1)`,
		`UPDATE wikit.sessions SET auth_version=auth_version+1 WHERE digest=$1`,
	} {
		if _, err := s.db.conn.Exec(ctx, change, identity.Digest(c.Session)); err != nil {
			t.Fatal(err)
		}
		s.req(t, "POST", "auth/session", "{}", c, "", 401)
		if _, err := s.db.conn.Exec(ctx, `UPDATE wikit.sessions SET expires_at=$1,auth_version=1 WHERE digest=$2`, time.Now().Add(time.Hour), identity.Digest(c.Session)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.conn.Exec(ctx, `UPDATE wikit.users SET status='active' WHERE id=$1`, s.admin.ID); err != nil {
			t.Fatal(err)
		}
	}
	c.CSRF = csrf
	s.req(t, "POST", "auth/logout", "{}", c, "", 204)
	s.req(t, "POST", "auth/session", "{}", c, "", 401)
}
