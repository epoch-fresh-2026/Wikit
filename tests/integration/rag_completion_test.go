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

func TestMCPRAGCompletedAndDistributedRequests(t *testing.T) {
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, admin, "public", document("蓝栎", "## 获得方式\n\n任务奖励。\n\n## 属性\n\n攻击间隔 2.4s\n\n阻挡数 2\n\n再部署时间 61s"), "completion-first")
	s.searchPage(t, admin, "public", document("灰槐", "## 获得方式\n\n兑换。\n\n## 属性\n\n攻击间隔 3.4s\n\n阻挡数 1\n\n再部署时间 62s"), "completion-second")
	s.searchPage(t, admin, "restricted", document("隐槐", "## 属性\n\n攻击间隔 9.4s private-completion-marker"), "completion-private")
	s.drain(t)
	_, token := s.register(t, "completion_reader")
	client, _ := s.connectMCP(t, token)
	for _, tc := range []struct {
		q, status string
		count     int
	}{
		{"请逐项核验蓝栎的阻挡数和攻击间隔资料。", "complete", 2},
		{"核实完蓝栎的获得方式后，请继续查询该干员的攻击间隔。", "complete", 2},
		{"请核查蓝栎的阻挡数，并把她的攻击间隔一并列出。", "complete", 2},
		{"请把蓝栎的获得方式确认好，再给出该干员的攻击间隔。", "complete", 2},
		{"请核对蓝栎和灰槐的获得方式，并提供两名干员各自的再部署时间。", "complete", 4},
		{"核查完蓝栎的远程攻击后，请继续提供该干员的攻击间隔。", "partial", 2},
		{"蓝栎和灰槐的获得方式与一名干员的再部署时间是什么？", "partial", 3},
		{"核实完隐槐的攻击间隔后，请继续查询她的阻挡数。", "insufficient", 2},
		{"请逐项核验未知银树的获得方式，并把她的攻击间隔列出。", "insufficient", 2},
	} {
		t.Run(tc.q, func(t *testing.T) {
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
			if out.RAG.Question != tc.q || out.RAG.Coverage.Status != tc.status || len(out.RAG.Coverage.Requirements) != tc.count {
				t.Fatal(out.RAG)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "private-completion-marker") {
				t.Fatal("private evidence")
			}
		})
	}
	for _, filter := range []map[string]any{{"type": "media"}, {"tag": "absent_completion_tag"}} {
		filter["q"] = "核实完蓝栎的获得方式后，请继续查询该干员的攻击间隔。"
		filter["mode"] = "rag"
		out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", filter, ""))
		if len(out.Items) > 0 || len(out.RAG.Evidence) > 0 || !slices.Contains(out.RAG.Coverage.MissingSubjects, "蓝栎") {
			t.Fatal("completion lost filter", out)
		}
	}
}
