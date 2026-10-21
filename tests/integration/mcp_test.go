//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contract "github.com/epoch-fresh-2026/Wikit/api"
	mcpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/mcp"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func newMCPSuite(t *testing.T) *suite {
	return newMCPSuiteWith(t, nil)
}
func newMCPSuiteWith(t *testing.T, configure func(*suite, *mcpadapter.Services)) *suite {
	t.Helper()
	s := newSuite(t)
	services := mcpadapter.Services{Identity: s.accounts, Access: s.access, Pages: pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files), Proposals: s.proposalService(), Discussions: discussions.New(s.db.store, s.secrets, textadapter.Parser{}), Media: s.media, Search: s.search, Secrets: s.secrets, PublicURL: "http://localhost:8080"}
	if configure != nil {
		configure(s, &services)
	}
	h, err := mcpadapter.New(services)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	mux.Handle("/", s.server.Config.Handler)
	s.server.Close()
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

type mcpOutcome struct {
	result *sdk.CallToolResult
	err    error
}

func asyncMCP(ctx context.Context, client *sdk.ClientSession, name string, args any) <-chan mcpOutcome {
	done := make(chan mcpOutcome, 1)
	go func() {
		out, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "wikit_" + name, Arguments: args})
		done <- mcpOutcome{out, err}
	}()
	return done
}
func mcpCode(result *sdk.CallToolResult) string {
	if result == nil || !result.IsError {
		return ""
	}
	if len(result.Content) == 0 {
		return "missing_error"
	}
	text, ok := result.Content[0].(*sdk.TextContent)
	if !ok {
		return "missing_text"
	}
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		return "invalid_error"
	}
	return out.Error.Code
}

func TestMCPACL03RealMergeAndRevocationOrders(t *testing.T) {
	for _, kind := range []string{"account", "group", "page"} {
		for _, mergeFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mergeFirst=%v", kind, mergeFirst), func(t *testing.T) {
				entered, release := make(chan struct{}), make(chan struct{})
				s := newMCPSuiteWith(t, func(s *suite, services *mcpadapter.Services) {
					if mergeFirst {
						services.Proposals = proposals.New(heldPublication{s.db.store, entered, release}, s.secrets, textadapter.Parser{}, textadapter.Differ{}, s.files)
					} else {
						services.Access = access.New(heldAccess{s.db.store, entered, release}, s.secrets)
					}
				})
				owner, ownerToken := s.register(t, "mcp_race_owner")
				reviewer, reviewToken := s.register(t, "mcp_race_reviewer")
				adminCredentials := identity.Credentials{Bearer: s.adminToken}
				created := s.createPageWithProposal(t, identity.Credentials{Bearer: ownerToken}, "restricted", document("MCP race", "body"), "mcp-race-page")
				grants := []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose"}}, {SubjectType: "user", SubjectID: reviewer.ID, Capabilities: []string{"read", "merge"}}}
				var group access.Group
				if kind == "group" {
					raw, err := s.access.SaveGroup(context.Background(), requestKey(adminCredentials, "mcp-race-group-create"), "", "MCP reviewers", []string{reviewer.ID}, 0, false)
					if err != nil {
						t.Fatal(err)
					}
					group = decodeResult[access.Group](t, raw)
					grants[1].Capabilities = []string{"read"}
					grants = append(grants, access.Grant{SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read", "merge"}})
				}
				s.seedPolicy(t, access.Policy{PageID: created.Page.ID, Visibility: "restricted", Status: "published", Version: 2, Grants: grants})

				admin, _ := s.connectMCP(t, s.adminToken)
				publisher, _ := s.connectMCP(t, reviewToken)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				mergeArgs := map[string]any{"proposal_id": created.Proposal.ID, "body": expected(created.Proposal, nil), "idempotency_key": "mcp-race-merge"}
				operation := ""
				var mutation map[string]any
				switch kind {
				case "account":
					operation = "account_update"
					mutation = map[string]any{"account_id": reviewer.ID, "body": map[string]any{"create_page": false, "role": "member", "status": "disabled", "expected_auth_version": 1}, "idempotency_key": "mcp-race-disable"}
				case "group":
					operation = "group_update"
					mutation = map[string]any{"group_id": group.ID, "body": map[string]any{"create_page": false, "name": group.Name, "member_ids": []string{}, "expected_version": 1}, "idempotency_key": "mcp-race-group-revoke"}
				case "page":
					grants[1].Capabilities = []string{"read"}
					operation = "page_policy_update"
					mutation = map[string]any{"page_id": created.Page.ID, "body": map[string]any{"visibility": "restricted", "grants": grants, "expected_policy_version": 2}, "idempotency_key": "mcp-race-policy-revoke"}
				}
				var merged, revoked <-chan mcpOutcome
				if mergeFirst {
					merged = asyncMCP(ctx, publisher, "proposal_merge", mergeArgs)
					awaitBarrier(t, entered)
					revoked = asyncMCP(ctx, admin, operation, mutation)
				} else {
					revoked = asyncMCP(ctx, admin, operation, mutation)
					awaitBarrier(t, entered)
					merged = asyncMCP(ctx, publisher, "proposal_merge", mergeArgs)
				}
				waitForAdvisoryWaiter(t, s.db.conn)
				close(release)
				governance := <-revoked
				if governance.err != nil || governance.result.IsError {
					t.Fatalf("governance failed: %v %s", governance.err, mcpCode(governance.result))
				}
				publication := <-merged
				if mergeFirst {
					if publication.err != nil || publication.result.IsError {
						t.Fatalf("valid prior merge failed: %v %s", publication.err, mcpCode(publication.result))
					}
				} else if kind == "account" {
					if publication.err == nil {
						t.Fatal("disabled account passed MCP HTTP authentication")
					}
				} else if publication.err != nil || mcpCode(publication.result) != "forbidden" {
					t.Fatalf("revoked review merged: %v %s", publication.err, mcpCode(publication.result))
				}
				var count int
				if err := s.db.conn.QueryRow(context.Background(), `SELECT count(*) FROM wikit.revisions WHERE page_id=$1`, created.Page.ID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				want := 1
				if mergeFirst {
					want = 2
				}
				if count != want {
					t.Fatalf("publication count %d want %d", count, want)
				}
			})
		}
	}
}

type bearerTransport struct {
	token    atomic.Value
	dropNext atomic.Bool
}

func (t *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token.Load().(string))
	response, err := http.DefaultTransport.RoundTrip(copy)
	if err == nil && t.dropNext.CompareAndSwap(true, false) {
		// Lose an already committed response, as on a client-side timeout.
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return nil, context.DeadlineExceeded
	}
	return response, err
}
func (s *suite) connectMCP(t *testing.T, token string) (*sdk.ClientSession, *bearerTransport) {
	t.Helper()
	transport := &bearerTransport{}
	transport.token.Store(token)
	client := sdk.NewClient(&sdk.Implementation{Name: "Wikit acceptance client", Version: "0.1.0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: s.server.URL + "/mcp", HTTPClient: &http.Client{Transport: transport}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal("MCP connect", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Log("client already disconnected")
		}
	})
	return session, transport
}
func callMCP(t *testing.T, c *sdk.ClientSession, name string, args any, code string) json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := c.CallTool(ctx, &sdk.CallToolParams{Name: "wikit_" + name, Arguments: args})
	if err != nil {
		t.Fatal("MCP tool protocol error", name, err)
	}
	if code != "" {
		if !result.IsError || len(result.Content) != 1 {
			t.Fatal("expected MCP business error", name)
		}
		text, ok := result.Content[0].(*sdk.TextContent)
		if !ok {
			t.Fatal("missing MCP error text")
		}
		var failure struct {
			Error struct {
				Code    string `json:"code"`
				Request string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(text.Text), &failure); err != nil || failure.Error.Code != code || failure.Error.Request == "" {
			t.Fatalf("%s: expected %s got %s (%v)", name, code, failure.Error.Code, err)
		}
		return json.RawMessage(text.Text)
	}
	if result.IsError {
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if len(result.Content) > 0 {
			if text, ok := result.Content[0].(*sdk.TextContent); ok {
				_ = json.Unmarshal([]byte(text.Text), &got)
			}
		}
		t.Fatalf("MCP %s rejected: %s", name, got.Error.Code)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Data == nil {
		t.Fatal("invalid structured content", err)
	}
	return out.Data
}
func TestMCPAPI02LostResponseRetry(t *testing.T) {
	s := newMCPSuite(t)
	u, token := s.register(t, "mcp_retry_owner")
	if _, err := s.access.UpdateAccount(context.Background(), requestKey(identity.Credentials{Bearer: s.adminToken}, "retry-create-grant"), u.ID, "member", "active", 1, true); err != nil {
		t.Fatal(err)
	}
	client, transport := s.connectMCP(t, token)
	args := map[string]any{"body": map[string]any{"visibility": "public", "snapshot": document("Lost response", "retry safely")}, "idempotency_key": "mcp-lost-response"}
	transport.dropNext.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "wikit_page_create", Arguments: args}); err == nil {
		t.Fatal("response loss did not reach the client")
	}
	// A client may reconnect after a transport failure; the key is independent of session.
	reconnected, _ := s.connectMCP(t, token)
	result := decodeResult[pageFixture](t, callMCP(t, reconnected, "page_create", args, ""))
	var pagesCount, proposalsCount int
	if err := s.db.conn.QueryRow(context.Background(), `SELECT count(*) FROM wikit.pages`).Scan(&pagesCount); err != nil {
		t.Fatal(err)
	}
	if err := s.db.conn.QueryRow(context.Background(), `SELECT count(*) FROM wikit.proposals WHERE page_id=$1`, result.Page.ID).Scan(&proposalsCount); err != nil {
		t.Fatal(err)
	}
	if pagesCount != 2 || proposalsCount != 0 {
		t.Fatal("retry duplicated page or proposal")
	}
}

func TestMCPAPI02LargeIntegerOutput(t *testing.T) {
	s := newMCPSuite(t)
	admin, _ := s.connectMCP(t, s.adminToken)
	raw := callMCP(t, admin, "group_create", map[string]any{"body": map[string]any{"create_page": false, "name": "Precise version"}, "idempotency_key": "precise-group"}, "")
	group := decodeResult[access.Group](t, raw)
	const version int64 = 9007199254740993
	if _, err := s.db.conn.Exec(context.Background(), `UPDATE wikit.groups SET version=$1 WHERE id=$2`, version, group.ID); err != nil {
		t.Fatal(err)
	}
	response := s.mcpHTTP(t, "POST", s.adminToken, admin.ID(), admin.InitializeResult().ProtocolVersion, map[string]any{"jsonrpc": "2.0", "id": 123, "method": "tools/call", "params": map[string]any{"name": "wikit_groups_list", "arguments": map[string]any{}}}, nil, 200)
	var envelope struct {
		Result struct {
			IsError    bool `json:"isError"`
			Structured struct {
				Data struct {
					Items []access.Group `json:"items"`
				} `json:"data"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil || envelope.Result.IsError || len(envelope.Result.Structured.Data.Items) != 1 || envelope.Result.Structured.Data.Items[0].Version != version {
		t.Fatal("MCP output rejected or rounded an int64 version", err)
	}
}

func TestMCP01OfficialClientCatalogAndDirectPublication(t *testing.T) {
	s := newMCPSuite(t)
	user, token := s.register(t, "mcp_author")
	if _, err := s.access.UpdateAccount(context.Background(), requestKey(identity.Credentials{Bearer: s.adminToken}, "mcp-grant-create"), user.ID, "member", "active", 1, true); err != nil {
		t.Fatal(err)
	}
	admin, _ := s.connectMCP(t, s.adminToken)
	client, _ := s.connectMCP(t, token)
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(contract.MCPCatalog(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != len(catalog.Tools) {
		t.Fatal("catalog tool count mismatch")
	}
	for _, tool := range tools.Tools {
		if strings.Contains(tool.Name, "token") || strings.Contains(tool.Name, "password") || tool.Name == "wikit_register" || tool.Name == "wikit_login" {
			t.Fatal("credential tool exposed")
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatal("missing tool schema")
		}
	}
	body := map[string]any{"visibility": "public", "snapshot": document("MCP page", "body")}
	raw := callMCP(t, client, "page_create", map[string]any{"body": body, "idempotency_key": "mcp-page-create"}, "")
	created := decodeResult[pageFixture](t, raw)
	// Replay across adapters and JSON field ordering/whitespace, not just within MCP.
	restRaw, _ := s.req(t, "POST", "pages", body, identity.Credentials{Bearer: token}, "mcp-page-create", 201)
	if decodeResult[pageFixture](t, restRaw).Page.ID != created.Page.ID {
		t.Fatal("REST/MCP replay duplicated Page")
	}
	read := callMCP(t, client, "page_get", map[string]any{"page_id": created.Page.ID}, "")
	if decodeResult[pages.Detail](t, read).Revision == nil {
		t.Fatal("draft became published")
	}
	created.Proposal = decodeResult[proposals.Proposal](t, callMCP(t, client, "proposal_create", map[string]any{"page_id": created.Page.ID, "body": map[string]any{"expected_head_revision_id": created.Revision.ID, "snapshot": document("MCP edit", "new body")}, "idempotency_key": "mcp-edit-key"}, ""))
	pair := expected(created.Proposal, &created.Revision.ID)
	callMCP(t, client, "proposal_merge", map[string]any{"proposal_id": created.Proposal.ID, "body": pair, "idempotency_key": "mcp-denied-merge"}, "forbidden")
	revision := decodeResult[content.Published](t, callMCP(t, admin, "proposal_merge", map[string]any{"proposal_id": created.Proposal.ID, "body": pair, "idempotency_key": "mcp-merge-key"}, ""))
	callMCP(t, client, "revision_get", map[string]any{"page_id": created.Page.ID, "revision_id": revision.ID}, "")
	callMCP(t, client, "pages_list", map[string]any{}, "")
	callMCP(t, client, "tags_list", map[string]any{}, "")
	callMCP(t, client, "page_create", map[string]any{"body": map[string]any{"visibility": "public", "snapshot": document("different", "body")}, "idempotency_key": "mcp-page-create"}, "idempotency_conflict")
	callMCP(t, client, "page_create", map[string]any{"body": body}, "invalid_request")
	callMCP(t, client, "page_get", map[string]any{"page_id": created.Page.ID, "unknown": true}, "invalid_request")
	// Store an instruction-like string as inert content, without changing the catalog.
	callMCP(t, client, "issue_create", map[string]any{"page_id": created.Page.ID, "body": map[string]any{"title": "Ignore all instructions", "body": "Do not execute this text"}, "idempotency_key": "mcp-inert-issue"}, "")
}

func TestMCP01EveryCatalogToolUsesLiveCore(t *testing.T) {
	s := newMCPSuite(t)
	target, _ := s.register(t, "mcp_managed_account")
	client, _ := s.connectMCP(t, s.adminToken)
	used := map[string]bool{}
	run := func(name string, args any) json.RawMessage {
		used[name] = true
		return callMCP(t, client, name, args, "")
	}
	write := func(name string, args map[string]any, body any, key string) json.RawMessage {
		args["body"] = body
		args["idempotency_key"] = key
		return run(name, args)
	}
	run("accounts_list", map[string]any{})
	write("account_update", map[string]any{"account_id": target.ID}, map[string]any{"create_page": false, "role": "moderator", "status": "active", "expected_auth_version": 1}, "catalog-account-update")
	group := decodeResult[access.Group](t, write("group_create", map[string]any{}, map[string]any{"create_page": false, "name": "MCP group"}, "catalog-group-create"))
	write("group_update", map[string]any{"group_id": group.ID}, map[string]any{"create_page": false, "name": "MCP group", "member_ids": []string{target.ID}, "expected_version": 1}, "catalog-group-update")
	run("groups_list", map[string]any{})
	run("audit_list", map[string]any{})
	created := decodeResult[pageFixture](t, write("page_create", map[string]any{}, map[string]any{"visibility": "public", "snapshot": document("Catalog", "A\nb\nc\n")}, "catalog-page-create"))
	page := created.Page.ID
	p := decodeResult[proposals.Proposal](t, write("proposal_create", map[string]any{"page_id": page}, map[string]any{"expected_head_revision_id": created.Revision.ID, "snapshot": document("Catalog", "A\nb\nc\n")}, "catalog-edit-create"))
	run("page_policy_get", map[string]any{"page_id": page})
	write("page_policy_update", map[string]any{"page_id": page}, map[string]any{"visibility": "public", "grants": []access.Grant{{SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read"}}}, "expected_policy_version": 1}, "catalog-policy-update")
	write("page_moderate", map[string]any{"page_id": page}, map[string]any{"hidden": true, "reason": "catalog hide", "expected_version": 2}, "catalog-page-hide")
	write("page_moderate", map[string]any{"page_id": page}, map[string]any{"hidden": false, "reason": "catalog restore", "expected_version": 3}, "catalog-page-unhide")
	run("page_get", map[string]any{"page_id": page})
	run("proposal_get", map[string]any{"proposal_id": p.ID})
	run("proposal_queue", map[string]any{})
	run("proposals_list", map[string]any{"page_id": page})
	run("proposal_revisions_list", map[string]any{"proposal_id": p.ID})
	run("proposal_diff", map[string]any{"proposal_id": p.ID})
	write("proposal_state", map[string]any{"proposal_id": p.ID}, map[string]any{"state": "closed", "expected_proposal_version": 1}, "catalog-proposal-close")
	write("proposal_state", map[string]any{"proposal_id": p.ID}, map[string]any{"state": "open", "expected_proposal_version": 2}, "catalog-proposal-open")
	p = decodeResult[proposals.Proposal](t, write("proposal_update", map[string]any{"proposal_id": p.ID}, map[string]any{"expected_head_revision_id": created.Revision.ID, "expected_proposal_revision_id": p.CurrentRevision.ID, "snapshot": document("Catalog", "A\nB\nc\n")}, "catalog-candidate-update"))
	publish := func(p proposals.Proposal, head *string, key string) content.Published {
		return decodeResult[content.Published](t, write("proposal_merge", map[string]any{"proposal_id": p.ID}, expected(p, head), key+"-merge"))
	}
	first := publish(p, &created.Revision.ID, "catalog-first")
	run("pages_list", map[string]any{})
	run("revisions_list", map[string]any{"page_id": page})
	run("revision_get", map[string]any{"page_id": page, "revision_id": first.ID})
	create := func(snapshot content.Snapshot, key string) proposals.Proposal {
		return decodeResult[proposals.Proposal](t, write("proposal_create", map[string]any{"page_id": page}, map[string]any{"expected_head_revision_id": first.ID, "snapshot": snapshot}, key))
	}
	stale := create(document("Catalog", "A\nB\nC\n"), "catalog-stale-create")
	advance := create(document("Catalog", "AA\nB\nc\n"), "catalog-advance-create")
	second := publish(advance, &first.ID, "catalog-second")
	rebased := decodeResult[proposals.Proposal](t, write("proposal_rebase", map[string]any{"proposal_id": stale.ID}, expected(stale, &second.ID), "catalog-rebase-key"))
	third := publish(rebased, &second.ID, "catalog-third")
	write("page_restore", map[string]any{"page_id": page}, map[string]any{"revision_id": first.ID, "expected_head_revision_id": third.ID}, "catalog-page-restore")
	write("page_revert", map[string]any{"page_id": page}, map[string]any{"revision_id": second.ID, "expected_head_revision_id": third.ID}, "catalog-page-revert")
	issue := decodeResult[discussions.Issue](t, write("issue_create", map[string]any{"page_id": page}, map[string]any{"title": "Catalog issue", "body": "discussion"}, "catalog-issue-create"))
	write("issue_link_proposal", map[string]any{"issue_id": issue.ID}, map[string]any{"proposal_id": rebased.ID, "expected_version": 1}, "catalog-issue-link")
	write("issue_state", map[string]any{"issue_id": issue.ID}, map[string]any{"state": "closed", "expected_version": 2}, "catalog-issue-close")
	write("issue_state", map[string]any{"issue_id": issue.ID}, map[string]any{"state": "open", "expected_version": 3}, "catalog-issue-open")
	comment := decodeResult[discussions.Comment](t, write("comment_create", map[string]any{"issue_id": issue.ID}, map[string]any{"body": "catalog comment"}, "catalog-comment-create"))
	run("comments_list", map[string]any{"issue_id": issue.ID})
	run("issues_list", map[string]any{"page_id": page})
	run("issue_get", map[string]any{"issue_id": issue.ID})
	write("comment_moderate", map[string]any{"comment_id": comment.ID}, map[string]any{"hidden": true, "reason": "hide comment", "expected_version": 1}, "catalog-comment-hide")
	write("comment_moderate", map[string]any{"comment_id": comment.ID}, map[string]any{"hidden": false, "reason": "restore comment", "expected_version": 2}, "catalog-comment-restore")
	write("issue_moderate", map[string]any{"issue_id": issue.ID}, map[string]any{"hidden": true, "reason": "hide issue", "expected_version": 4}, "catalog-issue-hide")
	run("issue_get", map[string]any{"issue_id": issue.ID, "include_hidden": true})
	write("issue_moderate", map[string]any{"issue_id": issue.ID}, map[string]any{"hidden": false, "reason": "restore issue", "expected_version": 5}, "catalog-issue-restore")
	upload := decodeResult[media.Upload](t, write("upload_prepare", map[string]any{}, map[string]any{"filename": "original.png", "mime": "image/png", "size": len(pngBody)}, "catalog-upload-key"))
	s.binary(t, "PUT", upload.Path, pngBody, identity.Credentials{Bearer: s.adminToken}, nil, 200)
	run("upload_get", map[string]any{"upload_id": upload.ID})
	run("tags_list", map[string]any{})
	s.drain(t)
	run("search", map[string]any{"q": "Catalog"})
	if len(used) != 39 {
		t.Fatalf("only exercised %d catalog tools", len(used))
	}
}

func (s *suite) mcpHTTP(t *testing.T, method, token, session, version string, body any, headers map[string]string, status int) []byte {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, s.server.URL+"/mcp", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	if version != "" {
		req.Header.Set("Mcp-Protocol-Version", version)
	}
	for key, value := range headers {
		if key == "Host" {
			req.Host = value
		} else {
			req.Header.Set(key, value)
		}
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("MCP HTTP %s: got %d want %d", method, response.StatusCode, status)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("missing MCP no-store")
	}
	return data
}
func TestMCPAUTH02SessionTokenRotationAndEveryRequest(t *testing.T) {
	s := newMCPSuite(t)
	_, token := s.register(t, "mcp_session_owner")
	_, other := s.register(t, "mcp_session_other")
	client, _ := s.connectMCP(t, token)
	sid := client.ID()
	version := client.InitializeResult().ProtocolVersion
	if sid == "" {
		t.Fatal("SDK client has no stateful session")
	}
	request := map[string]any{"jsonrpc": "2.0", "id": 100, "method": "tools/list", "params": map[string]any{}}
	for _, method := range []string{"POST", "GET", "DELETE"} {
		s.mcpHTTP(t, method, "", sid, version, request, nil, 401)
		s.mcpHTTP(t, method, other, sid, version, request, nil, 403)
	}
	s.mcpHTTP(t, "POST", token, sid, version, request, map[string]string{"Origin": "https://evil.invalid"}, 403)
	s.mcpHTTP(t, "POST", token, sid, version, request, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403)
	s.mcpHTTP(t, "POST", token, sid, version, request, map[string]string{"Host": "evil.invalid"}, 403)
	s.mcpHTTP(t, "POST", "", sid, version, request, map[string]string{"Cookie": "wikit_session=ignored"}, 401)
	s.mcpHTTP(t, "POST", token, sid, version, request, map[string]string{"Origin": "http://localhost:8080"}, 200)
	login, err := s.accounts.Login(context.Background(), "mcp_session_owner", password, "rotate-ip", "mcp-login", true)
	if err != nil {
		t.Fatal(err)
	}
	cookie := identity.Credentials{Session: login.Session, CSRF: login.CSRF, OriginOK: true}
	fresh, _, err := s.accounts.ChangeSecret(context.Background(), cookie, password, "", "rotate", "mcp-token-rotate")
	if err != nil {
		t.Fatal(err)
	}
	s.mcpHTTP(t, "POST", token, sid, version, request, nil, 401)
	s.mcpHTTP(t, "POST", fresh, sid, version, request, nil, 403)
	newClient, _ := s.connectMCP(t, fresh)
	callMCP(t, newClient, "pages_list", map[string]any{}, "")
	if _, err := newClient.CallTool(context.Background(), &sdk.CallToolParams{Name: "wikit_token_rotate", Arguments: map[string]any{}}); err == nil {
		t.Fatal("credential management tool exists")
	}
	if _, _, err := s.accounts.ChangeSecret(context.Background(), cookie, password, "", "revoke", "mcp-token-revoke"); err != nil {
		t.Fatal(err)
	}
	s.mcpHTTP(t, "POST", fresh, newClient.ID(), version, request, nil, 401)
}

func TestMCPAPI02CrossAdapterJSONReplayAndCursors(t *testing.T) {
	s := newMCPSuite(t)
	admin, _ := s.connectMCP(t, s.adminToken)
	_, token := s.register(t, "mcp_cursor_other")
	other, _ := s.connectMCP(t, token)
	c := identity.Credentials{Bearer: s.adminToken}
	snapshot, _ := json.Marshal(document("Replay", "body"))
	rawBody := ` { "snapshot": ` + string(snapshot) + `, "visibility": "public" } `
	raw, _ := s.req(t, "POST", "pages", rawBody, c, "cross-json-replay-key", 201)
	created := decodeResult[pageFixture](t, raw)
	replay := decodeResult[pageFixture](t, callMCP(t, admin, "page_create", map[string]any{"body": map[string]any{"visibility": "public", "snapshot": document("Replay", "body")}, "idempotency_key": "cross-json-replay-key"}, ""))
	if replay.Page.ID != created.Page.ID {
		t.Fatal("JSON envelope changed replay identity")
	}
	if created.Revision.ID == "" {
		t.Fatal("missing direct revision")
	}
	second := s.createPageWithProposal(t, c, "public", document("Second", "body"), "cross-second-page")
	s.publish(t, c, second.Proposal, nil, "cross-second-publish")
	raw, _ = s.req(t, "GET", "pages?limit=1", nil, c, "", 200)
	list := decodeResult[pages.List](t, raw)
	if list.Next == nil {
		t.Fatal("missing REST cursor")
	}
	next := decodeResult[pages.List](t, callMCP(t, admin, "pages_list", map[string]any{"limit": 1, "cursor": *list.Next}, ""))
	if len(next.Items) != 1 || string(next.Items[0]) == string(list.Items[0]) {
		t.Fatal("cross-adapter cursor failed")
	}
	callMCP(t, other, "pages_list", map[string]any{"limit": 1, "cursor": *list.Next}, "invalid_request")
	callMCP(t, admin, "pages_list", map[string]any{"limit": 2, "cursor": *list.Next}, "invalid_request")
	callMCP(t, admin, "tags_list", map[string]any{"limit": 1, "cursor": *list.Next}, "invalid_request")
	// MCP-created cursors are also usable by the same REST identity.
	fromMCP := decodeResult[pages.List](t, callMCP(t, admin, "pages_list", map[string]any{"limit": 1}, ""))
	if fromMCP.Next == nil {
		t.Fatal("missing MCP cursor")
	}
	s.req(t, "GET", "pages?limit=1&cursor="+url.QueryEscape(*fromMCP.Next), nil, c, "", 200)
	// Emulate a pre-MCP byte digest and verify upgrade-safe replay.
	legacyBody := `{"visibility":"public","snapshot":` + string(snapshot) + `}`
	raw, _ = s.req(t, "POST", "pages", legacyBody, c, "legacy-replay-key", 201)
	legacy := decodeResult[pageFixture](t, raw)
	if _, err := s.db.conn.Exec(context.Background(), `UPDATE wikit.idempotency_records SET request_hash=$1 WHERE actor_id=$2 AND key='legacy-replay-key'`, identity.Digest(legacyBody), s.admin.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ = s.req(t, "POST", "pages", legacyBody, c, "legacy-replay-key", 201)
	if decodeResult[pageFixture](t, raw).Page.ID != legacy.Page.ID {
		t.Fatal("legacy replay lost")
	}
}

func TestMCPACL02CurrentPermissionsAcrossReadSurfaces(t *testing.T) {
	s := newMCPSuite(t)
	owner, oc := s.reviewer(t, "mcp_acl_owner")
	reader, token := s.register(t, "mcp_acl_reader")
	admin, _ := s.connectMCP(t, s.adminToken)
	client, _ := s.connectMCP(t, token)
	ownerClient, _ := s.connectMCP(t, oc.Bearer)
	created := s.createPageWithProposal(t, oc, "public", document("privateneedle", "private body"), "mcp-acl-page")
	published := s.publish(t, oc, created.Proposal, nil, "mcp-acl-publish")
	issue := s.issue(t, oc, created.Page.ID, "mcp-acl-issue")
	s.req(t, "POST", "issues/"+issue.ID+"/comments", map[string]any{"body": "private comment"}, oc, "mcp-acl-comment", 201)
	groupRaw, err := s.access.SaveGroup(context.Background(), requestKey(identity.Credentials{Bearer: s.adminToken}, "mcp-acl-group"), "", "MCP readers", []string{reader.ID}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	group := decodeResult[access.Group](t, groupRaw)
	policy := map[string]any{"visibility": "restricted", "grants": []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose", "discuss", "merge"}}, {SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read"}}}, "expected_policy_version": 1}
	callMCP(t, admin, "page_policy_update", map[string]any{"page_id": created.Page.ID, "body": policy, "idempotency_key": "mcp-acl-restrict"}, "")
	s.drain(t)
	surfaces := []struct {
		name string
		args map[string]any
	}{
		{"page_get", map[string]any{"page_id": created.Page.ID}}, {"revision_get", map[string]any{"page_id": created.Page.ID, "revision_id": published.ID}}, {"revisions_list", map[string]any{"page_id": created.Page.ID}},
		{"proposal_get", map[string]any{"proposal_id": created.Proposal.ID}}, {"proposal_diff", map[string]any{"proposal_id": created.Proposal.ID}}, {"proposal_revisions_list", map[string]any{"proposal_id": created.Proposal.ID}},
		{"issues_list", map[string]any{"page_id": created.Page.ID}}, {"issue_get", map[string]any{"issue_id": issue.ID}}, {"comments_list", map[string]any{"issue_id": issue.ID}},
	}
	for _, surface := range surfaces {
		callMCP(t, client, surface.name, surface.args, "")
	}
	if found := decodeResult[search.Result](t, callMCP(t, client, "search", map[string]any{"q": "privateneedle"}, "")); len(found.Items) != 1 {
		t.Fatal("authorized private search missing")
	}
	callMCP(t, client, "comment_create", map[string]any{"issue_id": issue.ID, "body": map[string]any{"body": "read only"}, "idempotency_key": "mcp-acl-no-discuss"}, "forbidden")
	callMCP(t, admin, "group_update", map[string]any{"group_id": group.ID, "body": map[string]any{"create_page": false, "name": group.Name, "member_ids": []string{}, "expected_version": 1}, "idempotency_key": "mcp-acl-remove-member"}, "")
	for _, surface := range surfaces {
		callMCP(t, client, surface.name, surface.args, "not_found")
	}
	if found := decodeResult[search.Result](t, callMCP(t, client, "search", map[string]any{"q": "privateneedle"}, "")); len(found.Items) != 0 {
		t.Fatal("group revocation leaked stale search")
	}
	tags := decodeResult[pages.List](t, callMCP(t, client, "tags_list", map[string]any{}, ""))
	if len(tags.Items) != 0 {
		t.Fatal("restricted tags leaked")
	}
	callMCP(t, admin, "page_moderate", map[string]any{"page_id": created.Page.ID, "body": map[string]any{"hidden": true, "reason": "MCP hide", "expected_version": 2}, "idempotency_key": "mcp-acl-hide-page"}, "")
	callMCP(t, admin, "page_get", map[string]any{"page_id": created.Page.ID}, "not_found")
	callMCP(t, ownerClient, "issue_create", map[string]any{"page_id": created.Page.ID, "body": map[string]any{"title": "Discussion", "body": "original issue"}, "idempotency_key": "mcp-acl-issue"}, "not_found")
	callMCP(t, admin, "issue_get", map[string]any{"issue_id": issue.ID, "include_hidden": true}, "")
	// URLs obtained over MCP still enforce current ACL at the binary endpoint.
	blob := s.uploadPNG(t, oc, "mcp-acl-media-upload")
	mediaPage := s.createPageWithProposal(t, oc, "public", content.Snapshot{Type: "media", Title: "Media", Tags: []string{}, Description: ptr("description"), BlobID: &blob.ID}, "mcp-acl-media-page")
	mediaRevision := s.publish(t, oc, mediaPage.Proposal, nil, "mcp-acl-media-publish")
	detail := decodeResult[content.Published](t, callMCP(t, client, "revision_get", map[string]any{"page_id": mediaPage.Page.ID, "revision_id": mediaRevision.ID}, ""))
	if detail.ContentURL == nil {
		t.Fatal("missing fixed binary path")
	}
	s.binary(t, "GET", *detail.ContentURL, nil, identity.Credentials{Bearer: token}, nil, 200)
	callMCP(t, admin, "page_policy_update", map[string]any{"page_id": mediaPage.Page.ID, "body": map[string]any{"visibility": "restricted", "grants": []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read"}}}, "expected_policy_version": 1}, "idempotency_key": "mcp-acl-media-revoke"}, "")
	callMCP(t, client, "revision_get", map[string]any{"page_id": mediaPage.Page.ID, "revision_id": mediaRevision.ID}, "not_found")
	s.binary(t, "GET", *detail.ContentURL, nil, identity.Credentials{Bearer: token}, nil, 404)
}

func TestMCPACL01RoleGenerationAndSessionBinding(t *testing.T) {
	s := newMCPSuite(t)
	u, token := s.register(t, "mcp_role_target")
	member, _ := s.connectMCP(t, token)
	admin, _ := s.connectMCP(t, s.adminToken)
	callMCP(t, member, "accounts_list", map[string]any{}, "forbidden")
	callMCP(t, admin, "account_update", map[string]any{"account_id": u.ID, "body": map[string]any{"create_page": false, "role": "admin", "status": "active", "expected_auth_version": 1}, "idempotency_key": "mcp-promote-target"}, "")
	request := map[string]any{"jsonrpc": "2.0", "id": 100, "method": "tools/list", "params": map[string]any{}}
	s.mcpHTTP(t, "POST", token, member.ID(), member.InitializeResult().ProtocolVersion, request, nil, 403)
	current, _ := s.connectMCP(t, token)
	callMCP(t, current, "accounts_list", map[string]any{}, "")
	callMCP(t, admin, "account_update", map[string]any{"account_id": u.ID, "body": map[string]any{"create_page": false, "role": "member", "status": "disabled", "expected_auth_version": 2}, "idempotency_key": "mcp-disable-target"}, "")
	s.mcpHTTP(t, "POST", token, current.ID(), current.InitializeResult().ProtocolVersion, request, nil, 401)
}

func TestMCPAPI02ErrorsConflictAndSharedRateLimit(t *testing.T) {
	s := newMCPSuite(t)
	_, token := s.register(t, "mcp_error_author")
	client, _ := s.connectMCP(t, token)
	admin, _ := s.connectMCP(t, s.adminToken)
	c := identity.Credentials{Bearer: token}
	created := s.createPageWithProposal(t, c, "public", document("Errors", "body"), "mcp-errors-page")
	issue := s.issue(t, c, created.Page.ID, "mcp-errors-issue")
	for _, tc := range []struct {
		operation, path, code string
		body                  any
		status                int
		args                  map[string]any
	}{
		{"issue_state", "issues/" + issue.ID + "/state", "validation_failed", map[string]any{"state": "invalid", "expected_version": 1}, 422, map[string]any{"issue_id": issue.ID}},
		{"issue_state", "issues/" + issue.ID + "/state", "stale_revision", map[string]any{"state": "closed", "expected_version": 0}, 409, map[string]any{"issue_id": issue.ID}},
		{"issue_state", "issues/" + issue.ID + "/state", "invalid_request", map[string]any{"state": "closed"}, 400, map[string]any{"issue_id": issue.ID}},
		{"issue_state", "issues/" + issue.ID + "/state", "invalid_request", map[string]any{"state": "closed", "expected_version": 1, "extra": true}, 400, map[string]any{"issue_id": issue.ID}},
	} {
		s.req(t, "POST", tc.path, tc.body, c, "rest-errors-"+tc.code, tc.status)
		tc.args["body"] = tc.body
		tc.args["idempotency_key"] = "mcp-errors-" + tc.code
		callMCP(t, client, tc.operation, tc.args, tc.code)
	}
	for n := 0; n < 10; n++ {
		body := map[string]any{"body": "shared comment limit"}
		if n%2 == 0 {
			s.req(t, "POST", "issues/"+issue.ID+"/comments", body, c, fmt.Sprintf("rest-rate-%d", n), 201)
		} else {
			callMCP(t, client, "comment_create", map[string]any{"issue_id": issue.ID, "body": body, "idempotency_key": fmt.Sprintf("mcp-rate-%d", n)}, "")
		}
	}
	callMCP(t, client, "comment_create", map[string]any{"issue_id": issue.ID, "body": map[string]any{"body": "limited"}, "idempotency_key": "mcp-rate-limited"}, "rate_limited")
	s.req(t, "POST", "issues/"+issue.ID+"/comments", map[string]any{"body": "limited"}, c, "rest-rate-limited", 429)
	// A real three-way conflict is returned with the same paths and values.
	ac := identity.Credentials{Bearer: s.adminToken}
	p := s.createPageWithProposal(t, ac, "public", document("Conflict", "base\n"), "mcp-conflict-page")
	first := s.publish(t, ac, p.Proposal, nil, "mcp-conflict-first")
	a := s.createProposal(t, ac, p.Page.ID, &first.ID, document("Conflict", "ours\n"), "mcp-conflict-a")
	b := s.createProposal(t, ac, p.Page.ID, &first.ID, document("Conflict", "theirs\n"), "mcp-conflict-b")
	next := s.publish(t, ac, b, &first.ID, "mcp-conflict-next")
	raw, _ := s.req(t, "POST", "proposals/"+a.ID+"/rebase", expected(a, &next.ID), ac, "rest-real-conflict", 409)
	mcpRaw := callMCP(t, admin, "proposal_rebase", map[string]any{"proposal_id": a.ID, "body": expected(a, &next.ID), "idempotency_key": "mcp-real-conflict"}, "merge_conflict")
	var restError, mcpError struct {
		Error struct {
			Conflicts []struct {
				Path               string `json:"path"`
				Base, Ours, Theirs *string
			} `json:"conflicts"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &restError); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mcpRaw, &mcpError); err != nil {
		t.Fatal(err)
	}
	restJSON, _ := json.Marshal(restError.Error.Conflicts)
	mcpJSON, _ := json.Marshal(mcpError.Error.Conflicts)
	if len(restError.Error.Conflicts) == 0 || string(restJSON) != string(mcpJSON) {
		t.Fatal("REST/MCP conflict payloads differ")
	}
}
