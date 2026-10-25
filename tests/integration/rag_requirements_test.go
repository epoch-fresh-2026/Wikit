//go:build integration

package integration

import (
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"testing"
)

func TestMCPRAGMultiAttributeCoverage(t *testing.T) {
	s := newMCPSuite(t)
	c := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, c, "public", document("青柏", "## 属性\n\n攻击间隔 2.1s。\n\n## 获得方式\n\n任务奖励。"), "requirements-complete")
	s.searchPage(t, c, "public", document("白榆", "## 属性\n\n攻击间隔 3.2s。"), "requirements-partial")
	s.drain(t)
	client, _ := s.connectMCP(t, s.adminToken)
	for _, tc := range []struct {
		q, status string
		rows      int
	}{
		{"青柏的获得方式和攻击间隔分别是什么？", "complete", 2},
		{"白榆的获得方式和攻击间隔分别是什么？", "partial", 2},
		{"青柏和白榆的获得方式与攻击间隔分别是什么？", "partial", 4},
	} {
		out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
		if out.RAG.Coverage.Status != tc.status || len(out.RAG.Coverage.Requirements) != tc.rows {
			t.Fatalf("%s: %+v", tc.q, out.RAG)
		}
		for _, r := range out.RAG.Coverage.Requirements {
			if r.Subject == "白榆" && r.Attribute == "获得方式" && r.Status != "attribute_missing" {
				t.Fatal(r)
			}
			for _, id := range r.CitationIDs {
				found := false
				for _, e := range out.RAG.Evidence {
					if e.CitationID == id {
						found = true
					}
				}
				if !found {
					t.Fatal("coverage cited absent evidence", r)
				}
			}
		}
	}
}
