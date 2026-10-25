// Command retrieval-eval measures an explicitly supplied fixture against an
// already populated Wikit instance, read-only. It does not import test data.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type expected struct{ Operator, Section, Text string }
type question struct {
	ID, Question, Query     string
	Category, Answerability string
	Expected                []expected
	ExpectedSubjects        []string `json:"expected_subjects"`
	ExpectedMissingSubjects []string `json:"expected_missing_subjects"`
}
type fixture struct {
	Description     string
	SourceRevisions map[string]int `json:"source_revisions"`
	Cases           []question
}
type hit = search.Hit
type output = search.Result
type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	q := r.Clone(r.Context())
	q.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(q)
}

type measurement struct {
	ID                     string             `json:"id"`
	Track                  string             `json:"track"`
	Query                  string             `json:"query"`
	Hits                   []hit              `json:"hits"`
	Parity                 bool               `json:"rest_mcp_equal"`
	Recall                 float64            `json:"recall_at_5"`
	ReciprocalRank         float64            `json:"reciprocal_rank_at_5"`
	EvidenceRecall         float64            `json:"evidence_recall_at_5"`
	Precision              float64            `json:"returned_precision_at_5"`
	NegativeOK             bool               `json:"negative_ok"`
	RAG                    *search.RAGContext `json:"rag,omitempty"`
	CoverageCorrect        *bool              `json:"coverage_correct,omitempty"`
	StrictCoverageCorrect  *bool              `json:"strict_coverage_correct,omitempty"`
	SubjectsCorrect        *bool              `json:"subjects_correct,omitempty"`
	MissingSubjectsCorrect *bool              `json:"missing_subjects_correct,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: go run ./scripts/retrieval-eval FIXTURE_JSON OUTPUT_JSON")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var f fixture
	if err = json.Unmarshal(raw, &f); err != nil {
		return err
	}
	token := os.Getenv("WIKIT_LOCAL_MCP_TOKEN")
	if token == "" {
		return fmt.Errorf("WIKIT_LOCAL_MCP_TOKEN is required")
	}
	base := strings.TrimSuffix(os.Getenv("WIKIT_EVAL_URL"), "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	httpClient := &http.Client{Transport: bearer(token), Timeout: 20 * time.Second}
	client := sdk.NewClient(&sdk.Implementation{Name: "Wikit retrieval evaluation", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: base + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		return err
	}
	defer session.Close()
	results := []measurement{}
	for _, q := range f.Cases {
		for _, track := range []string{"question", "keywords", "rag_question", "rest_question"} {
			query := q.Query
			if track != "keywords" {
				query = q.Question
			}
			args := map[string]any{"q": query, "tags": []string{"PRTS样本"}, "type": "document", "limit": 5}
			if track == "rag_question" {
				args["mode"] = "rag"
			}
			res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "wikit_search", Arguments: args})
			if err != nil {
				return err
			}
			if res.IsError {
				return fmt.Errorf("%s MCP rejected: %v", q.ID, res.Content)
			}
			raw, err := json.Marshal(res.StructuredContent)
			if err != nil {
				return err
			}
			var envelope struct {
				Data search.RAGResult `json:"data"`
			}
			if err = json.Unmarshal(raw, &envelope); err != nil {
				return err
			}
			values := url.Values{"q": {query}, "tags": {"PRTS样本"}, "type": {"document"}, "limit": {"5"}}
			req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/v1/search?"+values.Encode(), nil)
			if err != nil {
				return err
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				return err
			}
			var rest output
			err = json.NewDecoder(resp.Body).Decode(&rest)
			resp.Body.Close()
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 {
				return fmt.Errorf("REST status %d", resp.StatusCode)
			}
			// Cursors are independently signed opaque continuations; compare all hit fields.
			m := measurement{ID: q.ID, Track: track, Query: query, Hits: envelope.Data.Items, Parity: reflect.DeepEqual(rest.Items, envelope.Data.Items)}
			if track == "rest_question" {
				m.Hits = rest.Items
			}
			if track == "rag_question" {
				m.RAG = &envelope.Data.RAG
				if len(m.RAG.Queries) == 0 || m.RAG.Question != q.Question || m.RAG.Instructions == "" {
					return fmt.Errorf("missing RAG context for %s", q.ID)
				}
				for _, e := range m.RAG.Evidence {
					matched := false
					for _, h := range m.Hits {
						if h.PageID != e.PageID || h.RevisionID != e.RevisionID {
							continue
						}
						for _, p := range h.Passages {
							if reflect.DeepEqual(p, e.Passage) {
								matched = true
							}
						}
					}
					if !matched {
						return fmt.Errorf("RAG citation outside returned evidence for %s", q.ID)
					}
				}
				var structuredValue, textValue any
				if err = json.Unmarshal(raw, &structuredValue); err != nil {
					return err
				}
				if len(res.Content) == 0 {
					return fmt.Errorf("RAG text content missing")
				}
				content, ok := res.Content[0].(*sdk.TextContent)
				if !ok {
					return fmt.Errorf("RAG text content missing")
				}
				if err = json.Unmarshal([]byte(content.Text), &textValue); err != nil {
					return err
				}
				if !reflect.DeepEqual(structuredValue, textValue) {
					return fmt.Errorf("RAG text and structured context differ")
				}
			}
			found, evidence := 0, 0
			for _, want := range q.Expected {
				for i, h := range m.Hits {
					if h.Title != want.Operator+"（明日方舟）" {
						continue
					}
					found++
					m.ReciprocalRank = max(m.ReciprocalRank, 1/float64(i+1))
					ps := h.Passages
					if m.RAG != nil {
						ps = []search.Passage{}
						for _, e := range m.RAG.Evidence {
							if e.PageID == h.PageID {
								ps = append(ps, e.Passage)
							}
						}
					}
					for _, p := range ps {
						if strings.Contains(p.Text, want.Text) && slices.Contains(p.SectionPath, want.Section) {
							evidence++
							break
						}
					}
					break
				}
			}
			if len(q.Expected) > 0 {
				m.Recall = float64(found) / float64(len(q.Expected))
				m.EvidenceRecall = float64(evidence) / float64(len(q.Expected))
				if len(m.Hits) > 0 {
					m.Precision = float64(found) / float64(len(m.Hits))
				}
			} else {
				m.NegativeOK = len(m.Hits) == 0
			}
			if m.RAG != nil {
				kind := q.Answerability
				if kind == "" {
					kind = "none"
					if len(q.Expected) > 0 {
						kind = "full"
					}
				}
				correct := false
				switch kind {
				case "full":
					correct = m.RAG.Coverage.Status == "complete" && m.EvidenceRecall == 1
				case "none":
					correct = m.RAG.Status == "insufficient_evidence" && m.RAG.Coverage.Status != "complete"
				case "partial":
					correct = m.RAG.Coverage.Status == "partial" && len(m.RAG.Coverage.MissingSubjects) > 0
				case "unsupported":
					for _, s := range m.RAG.Coverage.Subjects {
						if s.Status == "stage_unsupported" && m.RAG.Status == "insufficient_evidence" {
							correct = true
						}
					}
				}
				m.CoverageCorrect = &correct
				strict, subjects, missing := strictCoverage(q, m)
				m.StrictCoverageCorrect, m.SubjectsCorrect, m.MissingSubjectsCorrect = &strict, subjects, missing
			}
			results = append(results, m)
		}
	}
	metrics := map[string]any{}
	for _, track := range []string{"question", "keywords", "rag_question", "rest_question"} {
		n, negative, negativeOK, parity, exact := 0, 0, 0, 0, 0
		recall, mrr, evidence, precision := 0.0, 0.0, 0.0, 0.0
		for _, m := range results {
			if m.Track != track {
				continue
			}
			if m.Parity {
				parity++
			}
			var q question
			for _, v := range f.Cases {
				if v.ID == m.ID {
					q = v
					break
				}
			}
			if len(q.Expected) == 0 {
				negative++
				if m.NegativeOK {
					negativeOK++
				}
				continue
			}
			n++
			recall += m.Recall
			mrr += m.ReciprocalRank
			evidence += m.EvidenceRecall
			precision += m.Precision
			if m.Recall == 1 && m.Precision == 1 {
				exact++
			}
		}
		var parityMetric any = parity
		if track == "rag_question" {
			parityMetric = nil
		}
		metrics[track] = map[string]any{"answerable_cases": n, "mean_recall_at_5": recall / float64(n), "mrr_at_5": mrr / float64(n), "mean_evidence_recall_at_5": evidence / float64(n), "mean_returned_precision_at_5": precision / float64(n), "exact_result_sets": exact, "negative_cases": negative, "negative_correct": negativeOK, "rest_mcp_equal": parityMetric}
	}
	report := map[string]any{"evaluated_at": time.Now().UTC().Format(time.RFC3339), "fixture": f, "endpoint": base, "limit": 5, "metrics": metrics, "results": results}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(os.Args[2], append(raw, '\n'), 0600); err != nil {
		return err
	}
	summary, _ := json.MarshalIndent(metrics, "", "  ")
	fmt.Println(string(summary))
	for _, m := range results {
		if !m.Parity && m.Track != "rag_question" {
			return fmt.Errorf("REST/MCP mismatch for %s/%s", m.ID, m.Track)
		}
	}
	return nil
}

// Strict annotations are evaluation-only and never enter MCP arguments.
// Nil annotations preserve legacy fixtures; an explicit [] asserts no subjects.
func strictCoverage(q question, m measurement) (bool, *bool, *bool) {
	strict := m.CoverageCorrect != nil && *m.CoverageCorrect
	var subjects, missing *bool
	equalSet := func(a, b []string) bool {
		a, b = slices.Clone(a), slices.Clone(b)
		slices.Sort(a)
		slices.Sort(b)
		return slices.Equal(a, b)
	}
	if q.ExpectedSubjects != nil {
		ok := equalSet(q.ExpectedSubjects, m.RAG.Plan.Subjects)
		subjects = &ok
		strict = strict && ok
	}
	if q.ExpectedMissingSubjects != nil {
		ok := equalSet(q.ExpectedMissingSubjects, m.RAG.Coverage.MissingSubjects)
		for _, name := range q.ExpectedMissingSubjects {
			found := false
			for _, subject := range m.RAG.Coverage.Subjects {
				if subject.Subject == name && subject.Status == "not_found" {
					found = true
				}
			}
			ok = ok && found
		}
		missing = &ok
		strict = strict && ok
	}
	return strict, subjects, missing
}
