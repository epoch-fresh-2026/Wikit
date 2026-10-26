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

func TestMCPRAGCoordinatedRequestScope(t *testing.T) {
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, admin, "public", document("蓝栎", "## 获得方式\n\n任务奖励。\n\n## 属性\n\n攻击间隔 2.4s\n\n阻挡数 2\n\n再部署时间 61s"), "scope-first")
	s.searchPage(t, admin, "public", document("灰槐", "## 获得方式\n\n兑换。\n\n## 属性\n\n攻击间隔 3.4s\n\n阻挡数 1\n\n再部署时间 62s"), "scope-second")
	s.searchPage(t, admin, "restricted", document("隐槐", "## 属性\n\n攻击间隔 9.4s private-scope-marker"), "scope-private")
	s.drain(t)
	_, token := s.register(t, "scope_reader")
	client, _ := s.connectMCP(t, token)
	for _, tc := range []struct {
		q, status string
		count     int
	}{
		{"请核实蓝栎的阻挡数与攻击间隔资料。", "complete", 2},
		{"请把蓝栎的获得方式核实后，再给出该干员的攻击间隔。", "complete", 2},
		{"先确认蓝栎的阻挡数，再核对那位的攻击间隔资料。", "complete", 2},
		{"蓝栎的获得方式和上述干员的攻击间隔是什么？", "complete", 2},
		{"蓝栎和灰槐的获得方式与二者的再部署时间是什么？", "complete", 4},
		{"蓝栎的远程攻击和前述干员的攻击间隔是什么？", "partial", 2},
		{"蓝栎和灰槐的获得方式与上述干员的再部署时间是什么？", "partial", 3},
		{"请将隐槐的攻击间隔核查后，再返回上述干员的阻挡数。", "insufficient", 2},
		{"未知银树的获得方式及上述干员的攻击间隔是什么？", "insufficient", 2},
	} {
		t.Run(tc.q, func(t *testing.T) {
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
			if out.RAG.Question != tc.q || out.RAG.Coverage.Status != tc.status || len(out.RAG.Coverage.Requirements) != tc.count {
				t.Fatal(out.RAG)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "private-scope-marker") {
				t.Fatal("private evidence")
			}
		})
	}
	for _, filter := range []map[string]any{{"type": "media"}, {"tag": "absent_scope_tag"}} {
		filter["q"] = "请把蓝栎的获得方式核查后，再给出上述干员的攻击间隔。"
		filter["mode"] = "rag"
		out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", filter, ""))
		if len(out.Items) > 0 || len(out.RAG.Evidence) > 0 || !slices.Contains(out.RAG.Coverage.MissingSubjects, "蓝栎") {
			t.Fatal("scope lost filter", out)
		}
	}
}
