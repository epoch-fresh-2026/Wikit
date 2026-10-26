package search

import (
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
)

type blockText struct {
	Text
	blocks []TextBlock
}

func (blockText) NFC(s string) string                  { return s }
func (blockText) Fold(s string) string                 { return strings.ToLower(s) }
func (b blockText) Blocks(string) ([]TextBlock, error) { return b.blocks, nil }

func TestPassagesSelectLateEvidenceAndRetainCitation(t *testing.T) {
	revision := "33333333-3333-4333-8333-333333333333"
	section := "section-2"
	source := Source{Page: content.Page{ID: "5", HeadRevisionID: &revision}}
	parser := blockText{blocks: []TextBlock{
		{Text: "Unrelated opening", SectionPath: []string{}, StartLine: 1, EndLine: 1},
		{Text: strings.Repeat("前", 3000) + "LateNeedle is here" + strings.Repeat("后", 900), SectionPath: []string{"Stats"}, SectionID: &section, StartLine: 7, EndLine: 10},
	}}
	out, err := passages(source, "lateneedle", parser)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || !strings.Contains(out[0].Text, "LateNeedle") || len([]rune(out[0].Text)) > 600 || !out[0].Truncated || out[0].StartLine != 7 || out[0].EndLine != 10 || out[0].CitationURL != "/pages/5/revisions/"+revision+"#section-2" {
		t.Fatal(out)
	}
	out, err = passages(source, "metadataonly", parser)
	if err != nil || len(out) != 0 {
		t.Fatal("metadata must not fabricate evidence", out, err)
	}
}

func TestPassagesRankBodyBeforeSectionAndBoundResults(t *testing.T) {
	revision := "33333333-3333-4333-8333-333333333333"
	source := Source{Page: content.Page{ID: "5", HeadRevisionID: &revision}}
	parser := blockText{blocks: []TextBlock{}}
	for i := 0; i < 5; i++ {
		parser.blocks = append(parser.blocks, TextBlock{Text: "other", SectionPath: []string{"needle"}, StartLine: i + 1, EndLine: i + 1})
	}
	parser.blocks = append(parser.blocks, TextBlock{Text: "needle answer", SectionPath: []string{}, StartLine: 20, EndLine: 20})
	out, err := passages(source, "needle", parser)
	if err != nil || len(out) != 3 || out[0].Text != "needle answer" || out[1].StartLine != 1 {
		t.Fatal(out, err)
	}
}
