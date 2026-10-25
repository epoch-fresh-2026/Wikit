//go:build integration

package integration

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

func TestMCPRAGRequestDemonstratives(t *testing.T) {
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, admin, "public", document("丹槐", "## 获得方式\n\n任务奖励。\n\n## 属性\n\n攻击间隔 2.8s\n\n阻挡数 2"), "demo-first")
	s.searchPage(t, admin, "public", document("墨杉", "## 获得方式\n\n商店兑换。\n\n## 属性\n\n攻击间隔 3.8s\n\n阻挡数 1"), "demo-second")
	s.searchPage(t, admin, "restricted", document("隐杉", "## 获得方式\n\nprivate-demonstrative-marker\n\n## 属性\n\n攻击间隔 9.8s"), "demo-private")
	s.drain(t)
	_, token := s.register(t, "demonstrative_reader")
	client, _ := s.connectMCP(t, token)
	for _, tc := range []struct {
		q, status, subject, row string
		count                   int
	}{
		{"请核对丹槐的获得方式和这位干员的攻击间隔。", "complete", "丹槐", "covered", 2},
		{"我想了解丹槐的攻击间隔情况。", "complete", "丹槐", "covered", 1},
		{"请核实丹槐的远程攻击资料。", "insufficient", "丹槐", "attribute_missing", 1},
		{"丹槐和墨杉的获得方式以及这两位的阻挡数是什么？", "complete", "丹槐", "covered", 4},
		{"丹槐和墨杉的获得方式以及该干员的攻击间隔是什么？", "partial", "该干员", "unverified", 3},
		{"请核对这位干员的攻击间隔。", "insufficient", "这位干员", "unverified", 1},
		{"隐杉的获得方式和该干员的攻击间隔是什么？", "insufficient", "隐杉", "not_found", 2},
		{"未知紫槐的获得方式和那名角色的攻击间隔是什么？", "insufficient", "未知紫槐", "not_found", 2},
	} {
		t.Run(tc.q, func(t *testing.T) {
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
			if out.RAG.Question != tc.q || out.RAG.Coverage.Status != tc.status || len(out.RAG.Coverage.Requirements) != tc.count || !slices.ContainsFunc(out.RAG.Coverage.Requirements, func(r search.RequirementCoverage) bool { return r.Subject == tc.subject && r.Status == tc.row }) {
				t.Fatal(out.RAG)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "private-demonstrative-marker") {
				t.Fatal("private evidence")
			}
		})
	}
	for _, filter := range []map[string]any{{"type": "media"}, {"tag": "absent_demo_tag"}} {
		filter["q"] = "请核对丹槐的获得方式和该干员的攻击间隔。"
		filter["mode"] = "rag"
		out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", filter, ""))
		if len(out.Items) > 0 || len(out.RAG.Evidence) > 0 {
			t.Fatal("reference lost filter", out)
		}
	}
}
