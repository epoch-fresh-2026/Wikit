package search

import (
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
)

func TestRAGDeduplicatesBeforeBudgetWithoutChangingLexical(t *testing.T) {
	revision, section := "rev", "section-0"
	src := Source{Page: content.Page{ID: "2", HeadRevisionID: &revision}}
	parser := blockText{}
	for i := 1; i <= 3; i++ {
		parser.blocks = append(parser.blocks, TextBlock{Text: "retry introduction", SectionID: &section, StartLine: i, EndLine: i})
	}
	parser.blocks = append(parser.blocks, TextBlock{Text: "retry waits 42 seconds", SectionID: &section, StartLine: 9, EndLine: 9})
	rag, err := ragPassages(src, "retry", parser)
	if err != nil || len(rag) != 2 || rag[0].StartLine != 1 || rag[1].Text != "retry waits 42 seconds" || rag[1].StartLine != 9 {
		t.Fatal(rag, err)
	}
	lexical, err := passages(src, "retry", parser)
	if err != nil || len(lexical) != 3 || lexical[2].StartLine != 3 {
		t.Fatal(lexical, err)
	}
}

func TestRAGKeepsIndependentContextsAndTruncatedBlocks(t *testing.T) {
	revision, one, two := "rev", "section-0", "section-1"
	src := Source{Page: content.Page{ID: "2", HeadRevisionID: &revision}}
	for _, tc := range []struct {
		name   string
		blocks []TextBlock
	}{
		{"sections", []TextBlock{{Text: "needle 12", SectionID: &one}, {Text: "needle 12", SectionID: &two}}},
		{"long", []TextBlock{{Text: "needle " + strings.Repeat("x", 700) + "A", SectionID: &one}, {Text: "needle " + strings.Repeat("x", 700) + "B", SectionID: &one}}},
		{"case-sensitive-facts", []TextBlock{{Text: "needle secret=AbC", SectionID: &one}, {Text: "needle secret=abc", SectionID: &one}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ragPassages(src, "needle", blockText{blocks: tc.blocks})
			if err != nil || len(out) != 2 {
				t.Fatal(out, err)
			}
		})
	}
}
