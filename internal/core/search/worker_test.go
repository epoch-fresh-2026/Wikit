package search

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
)

type exportQueue struct {
	Queue
	source Source
}

func (q exportQueue) SearchSource(context.Context, string) (Source, error) { return q.source, nil }

type plainText struct{ Text }

func (plainText) Plain(s string) (string, error) { return s, nil }

type sizeLimitedIndex struct {
	Index
	requests int
}

func (i *sizeLimitedIndex) Put(_ context.Context, _ string, docs []Document) (int64, error) {
	raw, err := json.Marshal(docs)
	if err != nil {
		return 0, err
	}
	if len(raw) > 8<<20 {
		return 0, errors.New("transport request too large")
	}
	i.requests++
	return int64(i.requests), nil
}
func (*sizeLimitedIndex) Wait(context.Context, int64) error { return nil }

func TestRebuildExportsEscapedLargeDocuments(t *testing.T) {
	// Two individually accepted texts expand beyond the transport's request limit
	// when combined. A count-only batch loses the ability to rebuild this dataset.
	markdown := strings.Repeat("<", 1<<20)
	head := "00000000-0000-4000-8000-000000000001"
	q := exportQueue{source: Source{Page: content.Page{ID: head, Type: "document", Status: "published", HeadRevisionID: &head}, Snapshot: content.Snapshot{Title: "Large document", Type: "document", Tags: []string{}, Markdown: &markdown}}}
	index := &sizeLimitedIndex{}
	worker := Worker{index: index, text: plainText{}}
	if err := worker.export(context.Background(), q, "index", []string{head, head}); err != nil {
		t.Fatal(err)
	}
	if index.requests != 2 {
		t.Fatalf("expected individually accepted requests, got %d", index.requests)
	}
}
