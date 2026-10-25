package text

import (
	"slices"
	"strings"
	"testing"
)

func TestBlocksPreserveNestedSectionsAndSourceLines(t *testing.T) {
	markdown := "Intro\n\n# Root\n\n## Stats\n\nAttack **612** &amp; [label](https://private-target)\nsecond line\n\n#### Deep\n\n- Cost 18\n\n## Stats\n\nAgain\n\n<script>not evidence</script>\n"
	blocks, err := (Parser{}).Blocks(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("blocks: %+v", blocks)
	}
	if blocks[0].SectionID != nil || blocks[0].StartLine != 1 {
		t.Fatal(blocks[0])
	}
	stats := blocks[1]
	if !slices.Equal(stats.SectionPath, []string{"Root", "Stats"}) || *stats.SectionID != "section-1" || stats.StartLine != 7 || stats.EndLine != 8 || stats.Text != "Attack 612 & label second line" {
		t.Fatal(stats)
	}
	if *blocks[2].SectionID != "section-2" || !slices.Equal(blocks[2].SectionPath, []string{"Root", "Stats", "Deep"}) || blocks[2].StartLine != 12 {
		t.Fatal(blocks[2])
	}
	if *blocks[3].SectionID != "section-3" || !slices.Equal(blocks[3].SectionPath, []string{"Root", "Stats"}) {
		t.Fatal(blocks[3])
	}
	for _, block := range blocks {
		if strings.Contains(block.Text, "private-target") || strings.Contains(block.Text, "not evidence") {
			t.Fatal(block)
		}
	}
}

func TestBlocksTableCodeAndSetextHeading(t *testing.T) {
	blocks, err := (Parser{}).Blocks("Stats\n=====\n\n| Name | Attack |\n| --- | --- |\n| Amiya | 612 |\n\n```go\nneedle()\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 {
		t.Fatalf("%+v", blocks)
	}
	for _, block := range blocks {
		if block.SectionID == nil || *block.SectionID != "section-0" || block.StartLine < 4 || block.EndLine < block.StartLine {
			t.Fatal(block)
		}
	}
	if blocks[1].Text != "Amiya 612" || blocks[1].StartLine != 6 || blocks[1].EndLine != 6 || blocks[2].Text != "needle()" || blocks[2].StartLine != 9 {
		t.Fatal(blocks)
	}
}
