//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func TestAccountIDsDatabaseSequenceAndRollback(t *testing.T) {
	s := newSuite(t)
	if s.admin.ID != "1" {
		t.Fatal("local administrator must get first account ID", s.admin.ID)
	}
	u, _ := s.register(t, "numeric_first")
	if u.ID != "2" {
		t.Fatal("registration did not use next database ID", u.ID)
	}
	if _, _, err := s.accounts.Register(context.Background(), u.Username, password, "duplicate-ip", "duplicate", true); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	v, _ := s.register(t, "numeric_after_failure")
	if v.ID != "4" {
		t.Fatal("failed insertion must roll back account but may consume sequence", v.ID)
	}
	var accounts, tokens int
	if err := s.db.conn.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM wikit.users),(SELECT count(*) FROM wikit.user_tokens)").Scan(&accounts, &tokens); err != nil || accounts != 3 || tokens != 3 {
		t.Fatal("failed registration left partial account/token", accounts, tokens, err)
	}
	execSQL(t, s.db.conn, "SELECT setval('wikit.account_ids',9)")
	w, _ := s.register(t, "numeric_ten")
	if w.ID != "10" {
		t.Fatal("sequence did not cross decimal digit boundary")
	}
	admin := identity.Credentials{Bearer: s.adminToken}
	data, err := s.access.SaveGroup(context.Background(), requestKey(admin, "numeric-members-order"), "", "Ordered members", []string{w.ID, u.ID}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	g := decodeResult[access.Group](t, data)
	if len(g.Members) != 2 || g.Members[0] != "2" || g.Members[1] != "10" {
		t.Fatal("members not ordered by numeric ID", g.Members)
	}
}

func TestNumericAccountBaselineRejectsUUIDDatabaseWithoutChanges(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	execSQL(t, db.conn, `CREATE TABLE public.wikit_schema_migrations (
 version integer PRIMARY KEY, name text NOT NULL UNIQUE, checksum text NOT NULL,
 applied_at timestamptz NOT NULL DEFAULT now())`)
	for i, path := range []string{"../../internal/storage/postgres/migrations/0001_initialize.sql", "testdata/uuid_identity_access.sql"} {
		// The fixture is the actual UUID identity migration from the preceding
		// baseline. Record its own checksum, not a deliberately corrupted history.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		execSQL(t, db.conn, string(data))
		name := "migrations/0001_initialize.sql"
		if i == 1 {
			name = "migrations/0002_identity_access.sql"
		}
		if _, err := db.conn.Exec(ctx, "INSERT INTO public.wikit_schema_migrations(version,name,checksum) VALUES($1,$2,$3)", i+1, name, fmt.Sprintf("%x", sha256.Sum256(data))); err != nil {
			t.Fatal(err)
		}
	}
	execSQL(t, db.conn, `INSERT INTO wikit.users VALUES('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','legacy','legacy','Legacy','test-hash','admin','active',1,false)`)
	if err := db.store.Migrate(ctx); err == nil {
		t.Fatal("numeric baseline accepted UUID database")
	}
	if err := db.store.Check(ctx); err == nil {
		t.Fatal("UUID database reported ready")
	}
	var id string
	var migrations int
	if err := db.conn.QueryRow(ctx, "SELECT id::text,(SELECT count(*) FROM public.wikit_schema_migrations) FROM wikit.users WHERE username='legacy'").Scan(&id, &migrations); err != nil || id != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || migrations != 2 {
		t.Fatal("rejected legacy database was modified", id, migrations, err)
	}
}

func TestAccountIDsRESTMCPPrecisionAndValidation(t *testing.T) {
	s := newMCPSuite(t)
	ctx := context.Background()
	const large = "9007199254740993"
	execSQL(t, s.db.conn, "SELECT setval('wikit.account_ids',9007199254740992)")
	u, token := s.register(t, "large_numeric_id")
	if u.ID != large {
		t.Fatal("large account ID lost precision", u.ID)
	}
	raw, _ := s.req(t, "GET", "auth/me", nil, identity.Credentials{Bearer: token}, "", 200)
	if got := decodeResult[identity.Account](t, raw).ID; got != large {
		t.Fatal("REST account ID changed", got)
	}
	admin, _ := s.connectMCP(t, s.adminToken)
	update := func(id string) map[string]any {
		return map[string]any{"account_id": id, "idempotency_key": "numeric-account-update", "body": map[string]any{"role": "member", "status": "active", "create_page": true, "expected_auth_version": 1}}
	}
	out := callMCP(t, admin, "account_update", update(large), "")
	if got := decodeResult[access.AccountAdmin](t, out).Account.ID; got != large {
		t.Fatal("MCP account ID changed", got)
	}
	member, _ := s.connectMCP(t, token)
	created := decodeResult[pageFixture](t, callMCP(t, member, "page_create", map[string]any{"body": map[string]any{"visibility": "public", "snapshot": document("Numeric author", "Content")}, "idempotency_key": "numeric-page-create"}, ""))
	var author string
	if err := s.db.conn.QueryRow(ctx, "SELECT author_id::text FROM wikit.revisions WHERE id=$1", created.Revision.ID).Scan(&author); err != nil || author != large {
		t.Fatal("published author changed", author, err)
	}
	if created.Revision.AuthorID == nil || *created.Revision.AuthorID != large {
		t.Fatal("MCP published author lost precision")
	}
	login, err := s.accounts.Login(ctx, u.Username, password, "numeric-login-ip", "numeric-login", true)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = s.req(t, "GET", "auth/me", nil, identity.Credentials{Session: login.Session}, "", 200)
	if decodeResult[identity.Account](t, raw).ID != large {
		t.Fatal("Cookie identity changed")
	}
	group := decodeResult[access.Group](t, callMCP(t, admin, "group_create", map[string]any{"body": map[string]any{"name": "Numeric members", "create_page": false}, "idempotency_key": "numeric-group-create"}, ""))
	groupBody := map[string]any{"name": group.Name, "create_page": false, "member_ids": []string{large}, "expected_version": 1}
	callMCP(t, admin, "group_update", map[string]any{"group_id": group.ID, "body": groupBody, "idempotency_key": "numeric-group-update"}, "")
	groupBody["expected_version"] = 2
	grants := []access.Grant{{SubjectType: "user", SubjectID: large, Capabilities: []string{"read", "manage"}}, {SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read"}}}
	policyArgs := map[string]any{"page_id": created.Page.ID, "body": map[string]any{"visibility": "restricted", "grants": grants, "expected_policy_version": 1}, "idempotency_key": "numeric-mixed-policy"}
	callMCP(t, admin, "page_policy_update", policyArgs, "")
	policy := decodeResult[access.Policy](t, callMCP(t, admin, "page_policy_get", map[string]any{"page_id": created.Page.ID}, ""))
	if len(policy.Grants) != 2 {
		t.Fatal("mixed grants were lost")
	}
	for _, grant := range policy.Grants {
		if grant.SubjectType == "user" && grant.SubjectID != large || grant.SubjectType == "group" && grant.SubjectID != group.ID {
			t.Fatal("ACL subject changed")
		}
	}
	s.req(t, "GET", "pages/"+created.Page.ID, nil, identity.Credentials{Bearer: token}, "", 200)
	s.req(t, "GET", "pages/"+created.Page.ID, nil, identity.Credentials{}, "", 404)
	for i, id := range []string{"0", "01", "+1", "-1", "9223372036854775808", group.ID} {
		t.Run("invalid-"+strconv.Itoa(i), func(t *testing.T) {
			args := update(id)
			callMCP(t, admin, "account_update", args, "validation_failed")
			s.req(t, "PATCH", "admin/accounts/"+id, args["body"], identity.Credentials{Bearer: s.adminToken}, fmt.Sprintf("invalid-account-%d", i), 422)
			groupBody["member_ids"] = []string{id}
			callMCP(t, admin, "group_update", map[string]any{"group_id": group.ID, "body": groupBody, "idempotency_key": "invalid-numeric-member"}, "validation_failed")
		})
	}
	// Cross-kind IDs are rejected before storage; a valid group UUID cannot be a user.
	for _, grant := range []access.Grant{{SubjectType: "user", SubjectID: group.ID, Capabilities: []string{"read"}}, {SubjectType: "group", SubjectID: large, Capabilities: []string{"read"}}} {
		body := map[string]any{"visibility": "public", "grants": []access.Grant{grant}, "expected_policy_version": 2}
		callMCP(t, admin, "page_policy_update", map[string]any{"page_id": created.Page.ID, "body": body, "idempotency_key": "wrong-kind-policy"}, "validation_failed")
	}
	out = callMCP(t, admin, "audit_list", map[string]any{}, "")
	var events struct {
		Items []identity.Audit `json:"items"`
	}
	if err := json.Unmarshal(out, &events); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events.Items {
		if event.Action == "page.create" && event.ActorID != nil && *event.ActorID == large {
			found = true
		}
	}
	if !found {
		t.Fatal("numeric actor missing from audit")
	}
}
