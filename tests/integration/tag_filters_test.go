//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func TestTagFiltersRESTMCPParityAndPagination(t *testing.T) {
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	client, _ := s.connectMCP(t, s.adminToken)
	special := `quote"slash\, OR type = media`
	var ids []string
	for i, tags := range [][]string{{"alpha", "beta"}, {"alpha"}, {"beta", "old"}, {"gamma"}, {special, "alpha"}, {"café"}} {
		p := s.searchPage(t, admin, "public", content.Snapshot{Type: "document", Title: fmt.Sprintf("tagfilterneedle %d", i), Tags: tags, Markdown: ptr("tagfilterneedle body")}, fmt.Sprintf("tag-filter-page-%d", i))
		ids = append(ids, p.Page.ID)
	}
	s.drain(t)
	for _, tc := range []struct {
		name          string
		tags, exclude []string
		mode          string
		want          []string
	}{
		{"all", []string{" ALPHA ", "beta"}, nil, "all", ids[:1]},
		{"any", []string{"alpha", "beta"}, nil, "any", []string{ids[0], ids[1], ids[2], ids[4]}},
		{"combined", []string{"alpha", "beta"}, []string{"old"}, "any", []string{ids[0], ids[1], ids[4]}},
		{"only exclude", nil, []string{"old"}, "all", []string{ids[0], ids[1], ids[3], ids[4], ids[5]}},
		{"overlap", []string{"alpha", "beta"}, []string{"beta"}, "any", []string{ids[1], ids[4]}},
		{"impossible overlap", []string{"alpha"}, []string{"alpha"}, "all", []string{}},
		{"unicode", []string{"CAFE\u0301"}, nil, "all", ids[5:]},
		{"quoted", []string{special}, nil, "all", ids[4:5]},
		{"missing", []string{"missing"}, nil, "any", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, op := range []string{"pages_list", "search"} {
				path := "pages"
				values := url.Values{"tag_match": {tc.mode}, "type": {"document"}}
				args := map[string]any{"tag_match": tc.mode, "type": "document"}
				if tc.tags != nil {
					values["tags"] = tc.tags
					args["tags"] = tc.tags
				}
				if tc.exclude != nil {
					values["exclude_tags"] = tc.exclude
					args["exclude_tags"] = tc.exclude
				}
				if op == "search" {
					path = "search"
					values.Set("q", "tagfilterneedle")
					args["q"] = "tagfilterneedle"
				}
				raw, _ := s.req(t, "GET", path+"?"+values.Encode(), nil, admin, "", 200)
				rest := tagResultIDs(t, raw, op)
				mcp := tagResultIDs(t, callMCP(t, client, op, args, ""), op)
				// The bootstrap homepage participates in exclude-only page browsing.
				want := slices.Clone(tc.want)
				if tc.tags == nil && op == "pages_list" {
					want = append(want, "1")
				}
				slices.Sort(want)
				if !reflect.DeepEqual(rest, want) || !reflect.DeepEqual(mcp, want) {
					t.Fatalf("%s got REST=%v MCP=%v want=%v", op, rest, mcp, want)
				}
			}
		})
	}
	for _, path := range []string{"pages", "search"} {
		prefix := path + "?"
		if path == "search" {
			prefix += "q=tagfilterneedle&"
		}
		for _, bad := range []string{"tags=alpha&tag=alpha", "tags=&tag=", "tags=+", "exclude_tags=+", "tag_match=", "tag_match=or", "tag_match=all&tag_match=any", "type=document&type=media", "filter=visibility", "tags=" + url.QueryEscape(strings.Repeat("中", 65)), strings.Repeat("tags=alpha&", 31)} {
			s.req(t, "GET", prefix+bad, nil, admin, "", 400)
		}
		for _, bad := range []map[string]any{{"tag": "", "tags": []string{}}, {"tags": []string{" "}}, {"exclude_tags": []string{" "}}, {"tag_match": ""}, {"tag_match": "or"}, {"tags": make([]string, 31)}} {
			op := "pages_list"
			if path == "search" {
				op = "search"
				bad["q"] = "tagfilterneedle"
			}
			callMCP(t, client, op, bad, "invalid_request")
		}
		// Walk every page, reordering the OR operands on subsequent requests.
		var collected []string
		cursor := ""
		for n := 0; n < 10; n++ {
			values := url.Values{"tags": {"alpha", "beta"}, "tag_match": {"any"}, "exclude_tags": {"old"}, "limit": {"1"}}
			if path == "search" {
				values.Set("q", "tagfilterneedle")
			}
			if cursor != "" {
				values["tags"] = []string{" BETA ", "ALPHA", "alpha"}
				values.Set("cursor", cursor)
			}
			raw, _ := s.req(t, "GET", path+"?"+values.Encode(), nil, admin, "", 200)
			op := "pages_list"
			if path == "search" {
				op = "search"
			}
			collected = append(collected, tagResultIDs(t, raw, op)...)
			var result struct {
				Next *string `json:"next_cursor"`
			}
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.Next == nil {
				break
			}
			cursor = *result.Next
			changed := url.Values{}
			for key, v := range values {
				changed[key] = slices.Clone(v)
			}
			changed.Set("cursor", cursor)
			changed.Set("exclude_tags", "gamma")
			s.req(t, "GET", path+"?"+changed.Encode(), nil, admin, "", 400)
		}
		slices.Sort(collected)
		want := []string{ids[0], ids[1], ids[4]}
		slices.Sort(want)
		if !reflect.DeepEqual(collected, want) {
			t.Fatalf("pagination %s %v", path, collected)
		}
		s.req(t, "GET", prefix+"type=media&tags=alpha", nil, admin, "", 200)
	}
	// A stale index must not bypass current ACL after public access is revoked.
	keeper, _ := s.register(t, "tag_keeper")
	if _, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "tag-revoke"), ids[0], access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: keeper.ID, Capabilities: []string{"read"}}}}, 1); err != nil {
		t.Fatal(err)
	}
	_, token := s.register(t, "tag_reader")
	reader, _ := s.connectMCP(t, token)
	for _, op := range []string{"pages_list", "search"} {
		args := map[string]any{"tags": []string{"alpha", "beta"}, "tag_match": "all"}
		if op == "search" {
			args["q"] = "tagfilterneedle"
		}
		if got := tagResultIDs(t, callMCP(t, reader, op, args, ""), op); len(got) != 0 {
			t.Fatalf("revoked page leaked: %v", got)
		}
	}
}

func tagResultIDs(t *testing.T, raw json.RawMessage, op string) []string {
	t.Helper()
	ids := []string{}
	if op == "search" {
		for _, hit := range decodeResult[search.Result](t, raw).Items {
			ids = append(ids, hit.PageID)
		}
	} else {
		for _, item := range decodeResult[pages.List](t, raw).Items {
			var d pages.Detail
			if err := json.Unmarshal(item, &d); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, d.Page.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

// Return deliberately broad candidates to verify that the Core never trusts
// the index to enforce the requested tag or type predicate.
type broadTagIndex struct{ search.Index }

func (b broadTagIndex) Search(ctx context.Context, index, q string, f search.Filter, offset, limit int) (search.Candidates, error) {
	f.Tags = content.TagFilter{}
	f.Type = ""
	return b.Index.Search(ctx, index, q, f, offset, limit)
}

func TestTagFiltersTypeCurrentHeadAndCoreRecheck(t *testing.T) {
	s := newMCPSuite(t)
	c := identity.Credentials{Bearer: s.adminToken}
	client, _ := s.connectMCP(t, s.adminToken)
	doc := s.searchPage(t, c, "public", content.Snapshot{Type: "document", Title: "tagtypeneedle document", Tags: []string{"alpha", "beta"}, Markdown: ptr("body")}, "tagtype-document")
	s.searchPage(t, c, "public", content.Snapshot{Type: "document", Title: "tagtypeneedle excluded", Tags: []string{"alpha", "old"}, Markdown: ptr("body")}, "tagtype-excluded")
	blob := s.uploadPNG(t, c, "tagtype-upload")
	media := s.searchPage(t, c, "public", content.Snapshot{Type: "media", Title: "tagtypeneedle media", Tags: []string{"alpha", "beta"}, Description: ptr("body"), BlobID: &blob.ID}, "tagtype-media")
	s.drain(t)
	for _, op := range []string{"pages_list", "search"} {
		path := "pages"
		q := url.Values{"tags": {"alpha", "beta"}, "type": {"media"}}
		args := map[string]any{"tags": []string{"alpha", "beta"}, "type": "media"}
		if op == "search" {
			path = "search"
			q.Set("q", "tagtypeneedle")
			args["q"] = "tagtypeneedle"
		}
		raw, _ := s.req(t, "GET", path+"?"+q.Encode(), nil, c, "", 200)
		if got := tagResultIDs(t, raw, op); !reflect.DeepEqual(got, []string{media.Page.ID}) {
			t.Fatalf("type REST %v", got)
		}
		if got := tagResultIDs(t, callMCP(t, client, op, args, ""), op); !reflect.DeepEqual(got, []string{media.Page.ID}) {
			t.Fatalf("type MCP %v", got)
		}
	}
	broad := search.New(s.db.store, broadTagIndex{s.index}, textadapter.Parser{}, s.secrets)
	for _, f := range []content.TagFilter{
		{Tags: []string{"alpha", "beta"}},
		{Tags: []string{"alpha", "gamma"}, Match: "any", ExcludeTags: []string{"old"}},
		{ExcludeTags: []string{"old"}},
	} {
		out, err := broad.Search(context.Background(), c, search.Query{Q: "tagtypeneedle", Type: "document", Filter: f, Limit: 1})
		if err != nil || len(out.Items) != 1 || out.Items[0].PageID != doc.Page.ID {
			t.Fatalf("core recheck %+v %v", out, err)
		}
	}
	// Legacy and new single-label input share canonical cursor bindings.
	for _, op := range []string{"pages_list", "search"} {
		args := map[string]any{"tag": "ALPHA", "limit": 1}
		if op == "search" {
			args["q"] = "tagtypeneedle"
		}
		raw := callMCP(t, client, op, args, "")
		var result struct {
			Next *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Next == nil {
			t.Fatal("missing legacy cursor")
		}
		delete(args, "tag")
		args["tags"] = []string{"alpha", "ALPHA"}
		args["tag_match"] = "any"
		args["cursor"] = *result.Next
		callMCP(t, client, op, args, "")
	}
	replacement := s.createProposal(t, c, doc.Page.ID, doc.Page.HeadRevisionID, content.Snapshot{Type: "document", Title: "tagtypeneedle document", Tags: []string{"gamma"}, Markdown: ptr("body")}, "tagtype-replace")
	s.publish(t, c, replacement, doc.Page.HeadRevisionID, "tagtype-replace-merge")
	out, err := broad.Search(context.Background(), c, search.Query{Q: "tagtypeneedle", Type: "document", Filter: content.TagFilter{Tags: []string{"alpha"}, ExcludeTags: []string{"old"}}, Limit: 20})
	if err != nil || len(out.Items) != 0 {
		t.Fatalf("old tags leaked %+v %v", out, err)
	}
	s.drain(t)
	out, err = s.search.Search(context.Background(), c, search.Query{Q: "tagtypeneedle", Filter: content.TagFilter{Tags: []string{"gamma"}}, Limit: 20})
	if err != nil || len(out.Items) != 1 || out.Items[0].PageID != doc.Page.ID {
		t.Fatalf("updated tags missing %+v %v", out, err)
	}
	callMCP(t, client, "page_moderate", map[string]any{"page_id": doc.Page.ID, "body": map[string]any{"hidden": true, "reason": "test tag filtering", "expected_version": 1}, "idempotency_key": "tagtype-hide"}, "")
	for _, op := range []string{"pages_list", "search"} {
		args := map[string]any{"tags": []string{"gamma"}}
		if op == "search" {
			args["q"] = "tagtypeneedle"
		}
		if got := tagResultIDs(t, callMCP(t, client, op, args, ""), op); len(got) != 0 {
			t.Fatal("hidden page leaked", got)
		}
	}
}
