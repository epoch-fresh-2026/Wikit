//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
	"strings"
	"testing"
)

func TestMCPRAGClauseBoundCoverage(t *testing.T) {
	s := newMCPSuite(t)
	c := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, c, "public", document("青柏", "## 获得方式\n\n任务奖励。\n\n## 属性\n\n精英0 精英1 精英2 生命上限 10 20 30"), "binding-first")
	s.searchPage(t, c, "public", document("白榆", "## 属性\n\n攻击间隔 3.2s。\n\n精英0 精英1 精英2 攻击 40 50 ——"), "binding-second")
	s.searchPage(t, c, "restricted", document("黑栎", "## 属性\n\n攻击间隔 9.2s private-binding-marker"), "binding-private")
	s.drain(t)
	_, token := s.register(t, "binding_reader")
	client, _ := s.connectMCP(t, token)
	for _, tc := range []struct {
		q, status string
		rows      int
	}{
		{"青柏的获得方式和白榆的攻击间隔分别是什么？", "complete", 2},
		{"青柏和白榆的获得方式与攻击间隔分别是什么？", "partial", 4},
		{"青柏精英二的生命上限和白榆精英一的攻击是多少？", "complete", 2},
		{"青柏精英一的生命上限和白榆精英二的攻击是多少？", "partial", 2},
		{"青柏的获得方式和黑栎的攻击间隔分别是什么？", "partial", 2},
		{"青柏的获得方式和不存在树的攻击间隔分别是什么？", "partial", 2},
	} {
		t.Run(tc.q, func(t *testing.T) {
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
			if out.RAG.Coverage.Status != tc.status || len(out.RAG.Coverage.Requirements) != tc.rows || len(out.RAG.Plan.Bindings) != tc.rows {
				t.Fatal(out.RAG)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "private-binding-marker") {
				t.Fatal("private content returned")
			}
		})
	}
}

func TestRAGBoundFinalAuthorization(t *testing.T) {
	s := newSearchSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	_, token := s.register(t, "bound_final_reader")
	first := s.searchPage(t, admin, "public", document("青柏", "## 获得方式\n\n任务奖励 bound-secret"), "bound-final-first")
	s.searchPage(t, admin, "public", document("白榆", "## 属性\n\n攻击间隔 3.2s"), "bound-final-second")
	s.drain(t)
	repo := &beforeRAGFinal{Repository: s.db.store, mutate: func() {
		s.req(t, "POST", "pages/"+first.Page.ID+"/moderation", map[string]any{"hidden": true, "reason": "bound final hide", "expected_version": 1}, admin, "bound-final-hide", 200)
	}}
	svc := search.New(repo, s.index, textadapter.Parser{}, s.secrets)
	out, err := svc.Retrieve(context.Background(), identity.Credentials{Bearer: token}, search.Query{Q: "青柏的获得方式和白榆的攻击间隔分别是什么？", Limit: 5})
	if err != nil || repo.calls != 7 || out.RAG.Coverage.Status != "partial" || len(out.Items) != 1 || out.Items[0].Title != "白榆" {
		t.Fatal(out, err, repo.calls)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "bound-secret") {
		t.Fatal("stale authorization returned evidence")
	}
}
