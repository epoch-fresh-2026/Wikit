//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

// TestMCPRAGPipelineDataset uses the same frozen corpus and evaluator before
// and after optimization. Optional output records every failure, never skips it.
func TestMCPRAGPipelineDataset(t *testing.T) {
	var f struct {
		Documents []struct{ Title, Body, Visibility string }
		Cases     []struct {
			ID, Question, Status string
			Limit                int
			Expected, Subjects   []string
			EvidenceCount        *int `json:"evidence_count"`
			Truncated            *bool
		}
	}
	raw, err := os.ReadFile("../fixtures/rag-pipeline.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	s := newMCPSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	author, authorToken := s.register(t, "pipeline_author")
	if _, err := s.db.conn.Exec(context.Background(), "UPDATE wikit.users SET create_page=true WHERE id=$1", author.ID); err != nil {
		t.Fatal(err)
	}
	pageService := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	for i, d := range f.Documents {
		visibility := d.Visibility
		if visibility == "" {
			visibility = "public"
		}
		key := fmt.Sprintf("pipeline-%d", i)
		// Two authors seed ten pages each within the real production rate limit.
		credentials := admin
		if i >= 10 {
			credentials = identity.Credentials{Bearer: authorToken}
		}
		if _, err := pageService.Create(context.Background(), requestKey(credentials, key), visibility, document(d.Title, d.Body), key); err != nil {
			t.Fatal(err)
		}
	}
	s.drain(t)
	_, token := s.register(t, "pipeline_reader")
	client, _ := s.connectMCP(t, token)
	type measurement struct {
		ID                string `json:"id"`
		Passed            bool   `json:"passed"`
		Matched, Expected int
		LatencyMS         float64          `json:"latency_ms"`
		Result            search.RAGResult `json:"result"`
	}
	results := []measurement{}
	passed, matched, expected := 0, 0, 0
	for _, tc := range f.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			start := time.Now()
			out := decodeResult[search.RAGResult](t, callMCP(t, client, "search", map[string]any{"q": tc.Question, "mode": "rag", "limit": tc.Limit}, ""))
			m := measurement{ID: tc.ID, Passed: true, Expected: len(tc.Expected), LatencyMS: float64(time.Since(start).Microseconds()) / 1000, Result: out}
			texts := []string{}
			for _, e := range out.RAG.Evidence {
				texts = append(texts, e.Passage.Text)
				if !strings.HasPrefix(e.Passage.CitationURL, "/pages/"+e.PageID+"/revisions/"+e.RevisionID) || e.Passage.StartLine < 1 || e.Passage.EndLine < e.Passage.StartLine || len([]rune(e.Passage.Text)) > 600 {
					t.Error("invalid evidence provenance or budget", e)
					m.Passed = false
				}
			}
			for _, want := range tc.Expected {
				if slices.ContainsFunc(texts, func(v string) bool { return strings.Contains(v, want) }) {
					m.Matched++
				} else {
					t.Error("missing evidence", want)
					m.Passed = false
				}
			}
			for _, subject := range tc.Subjects {
				if !slices.ContainsFunc(out.Items, func(h search.Hit) bool { return h.Title == subject }) {
					t.Error("missing selected subject", subject)
					m.Passed = false
				}
			}
			if tc.EvidenceCount != nil && len(texts) != *tc.EvidenceCount {
				t.Error("evidence count", len(texts), "want", *tc.EvidenceCount)
				m.Passed = false
			}
			if tc.Truncated != nil && out.RAG.Coverage.Truncated != *tc.Truncated {
				t.Error("truncation", out.RAG.Coverage.Truncated, "want", *tc.Truncated)
				m.Passed = false
			}
			if tc.Status != "" && out.RAG.Status != tc.Status {
				t.Error("status", out.RAG.Status, "want", tc.Status)
				m.Passed = false
			}
			if len(texts) > 16 {
				t.Error("evidence budget exceeded")
				m.Passed = false
			}
			if m.Passed {
				passed++
			}
			matched += m.Matched
			expected += m.Expected
			results = append(results, m)
		})
	}
	t.Logf("pipeline dataset: cases=%d passed=%d evidence=%d/%d", len(results), passed, matched, expected)
	if path := os.Getenv("WIKIT_PIPELINE_REPORT"); path != "" {
		report := struct {
			Cases, Passed, Matched, Expected int
			Results                          []measurement
		}{len(results), passed, matched, expected, results}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
