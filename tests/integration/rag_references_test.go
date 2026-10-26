//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func TestMCPRAGReferenceAndIdentity(t *testing.T) {
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, admin, "public", document("紫桐", "## 获得方式\n\n任务奖励。\n\n## 属性\n\n攻击间隔 2.7s"), "ref-first")
	s.searchPage(t, admin, "public", document("黄榆", "## 特性\n\n可以远程攻击。"), "ref-second")
	s.searchPage(t, admin, "restricted", document("黑槐", "## 获得方式\n\nprivate-reference-marker"), "ref-hidden")
	s.drain(t)
	_, token := s.register(t, "reference_reader")
	client, _ := s.connectMCP(t, token)
	for _, filter := range []map[string]any{{"type": "media"}, {"tag": "absent_reference_tag"}} {
		filter["q"] = "紫桐的远程攻击是什么？"
		filter["mode"] = "rag"
		out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", filter, ""))
		if len(out.Items) > 0 || len(out.RAG.Evidence) > 0 || !slices.Contains(out.RAG.Coverage.MissingSubjects, "紫桐") {
			t.Fatal("identity probe lost filter", out)
		}
	}
	for _, tc := range []struct{ q, coverage, subject, attribute, status string }{
		{"紫桐的获得方式和她的攻击间隔是什么？", "complete", "紫桐", "攻击间隔", "covered"},
		{"紫桐的远程攻击和黄榆的特性是什么？", "partial", "紫桐", "远程攻击", "attribute_missing"},
		{"紫桐的远程攻击是什么？", "insufficient", "紫桐", "远程攻击", "attribute_missing"},
		{"紫桐的获得方式和黑槐的攻击间隔是什么？", "partial", "黑槐", "攻击间隔", "not_found"},
		{"紫桐的获得方式和不存在树的攻击间隔是什么？", "partial", "不存在树", "攻击间隔", "not_found"},
		{"紫桐和黄榆的特性和她的攻击间隔是什么？", "partial", "她", "攻击间隔", "unverified"},
		{"紫桐、无枝甲、无枝乙、无枝丙、无枝丁的攻击间隔是多少？", "partial", "无枝丁", "攻击间隔", "unverified"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
			if out.RAG.Question != tc.q || out.RAG.Coverage.Status != tc.coverage || len(out.RAG.Queries) > 4 {
				t.Fatal(out.RAG)
			}
			if !slices.ContainsFunc(out.RAG.Coverage.Requirements, func(r search.RequirementCoverage) bool {
				return r.Subject == tc.subject && r.Attribute == tc.attribute && r.Status == tc.status
			}) {
				t.Fatal(out.RAG)
			}
			if tc.status != "not_found" && slices.Contains(out.RAG.Coverage.MissingSubjects, tc.subject) {
				t.Fatal(out.RAG)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "private-reference-marker") {
				t.Fatal("private evidence")
			}
		})
	}
}

func TestRAGIdentityProbeFinalAuthorization(t *testing.T) {
	s := newSearchSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	_, token := s.register(t, "reference_final_reader")
	first := s.searchPage(t, admin, "public", document("紫桐", "## 获得方式\n\nidentity-secret"), "ref-final-first")
	s.searchPage(t, admin, "public", document("黄榆", "## 属性\n\n攻击间隔 2.7s"), "ref-final-second")
	s.drain(t)
	repo := &beforeRAGFinal{Repository: s.db.store, mutate: func() {
		s.req(t, "POST", "pages/"+first.Page.ID+"/moderation", map[string]any{"hidden": true, "reason": "identity final hide", "expected_version": 1}, admin, "ref-final-hide", 200)
	}}
	svc := search.New(repo, s.index, textadapter.Parser{}, s.secrets)
	out, err := svc.Retrieve(context.Background(), identity.Credentials{Bearer: token}, search.Query{Q: "紫桐的攻击间隔和黄榆的攻击间隔是什么？", Limit: 5})
	if err != nil || repo.calls != 7 || len(out.Items) != 1 || out.Items[0].Title != "黄榆" || out.RAG.Coverage.Status != "partial" {
		t.Fatal(out, err, repo.calls)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "identity-secret") {
		t.Fatal("stale identity evidence")
	}
}
