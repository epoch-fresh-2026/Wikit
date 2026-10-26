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

func TestMCPRAGNormalizedRequestObjects(t *testing.T) {
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	s.searchPage(t, admin, "public", document("蓝栎", "## 获得方式\n\n任务奖励。\n\n## 属性\n\n攻击间隔 2.4s\n\n阻挡数 2\n\n再部署时间 61s"), "objects-first")
	s.searchPage(t, admin, "public", document("灰槐", "## 获得方式\n\n兑换。\n\n## 属性\n\n攻击间隔 3.4s\n\n阻挡数 1\n\n再部署时间 62s"), "objects-second")
	s.searchPage(t, admin, "restricted", document("隐槐", "## 属性\n\n攻击间隔 9.4s private-objects-marker"), "objects-private")
	s.drain(t)
	_, token := s.register(t, "objects_reader")
	client, _ := s.connectMCP(t, token)
	for _, tc := range []struct {
		q, status string
		count     int
	}{
		{"请给出蓝栎和灰槐的阻挡数，再把两名角色各自的获得方式列出。", "complete", 4},
		{"把蓝栎的获得方式核实清楚后，请继续查询她的攻击间隔。", "complete", 2},
		{"请核实蓝栎的阻挡数，再将其攻击间隔一并列出。", "complete", 2},
		{"请先完成蓝栎获得方式的核验，再查询他的攻击间隔。", "complete", 2},
		{"请逐条核对蓝栎的获得方式，并顺便查询该干员的再部署时间。", "complete", 2},
		{"请核验蓝栎的获得方式，接下来继续给出其攻击间隔。", "complete", 2},
		{"请列出蓝栎和灰槐的获得方式，然后补充这二人的再部署时间。", "complete", 4},
		{"请完成蓝栎远程攻击的核验，再查询其攻击间隔。", "partial", 2},
		{"蓝栎和灰槐的获得方式与其再部署时间是什么？", "partial", 3},
		{"请核实隐槐的攻击间隔，接下来继续给出其阻挡数。", "insufficient", 2},
		{"请完成未知银树的获得方式的核验，再补充其攻击间隔。", "insufficient", 2},
	} {
		t.Run(tc.q, func(t *testing.T) {
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.q, "mode": "rag"}, ""))
			if out.RAG.Question != tc.q || out.RAG.Coverage.Status != tc.status || len(out.RAG.Coverage.Requirements) != tc.count {
				t.Fatal(out.RAG)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "private-objects-marker") {
				t.Fatal("private evidence")
			}
		})
	}
	for _, filter := range []map[string]any{{"type": "media"}, {"tag": "absent_objects_tag"}} {
		filter["q"] = "把蓝栎的获得方式核实清楚后，请继续查询她的攻击间隔。"
		filter["mode"] = "rag"
		out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", filter, ""))
		if len(out.Items) > 0 || len(out.RAG.Evidence) > 0 || !slices.Contains(out.RAG.Coverage.MissingSubjects, "蓝栎") {
			t.Fatal("objects lost filter", out)
		}
	}
}
