package text

import (
	"reflect"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
)

func TestTagFilterUsesPublishedTagNormalization(t *testing.T) {
	parser := Parser{}
	for _, raw := range []string{"ǰ", "j\u030c", "ΐ", "ι\u0308\u0301", strings.Repeat("ǰ", 64)} {
		t.Run(raw, func(t *testing.T) {
			snapshot, _, err := content.Normalize(parser, content.Snapshot{Type: "document", Title: "Unicode", Tags: []string{raw}, Markdown: stringPtr("body")})
			if err != nil {
				t.Fatal(err)
			}
			included, err := (content.TagFilter{Tags: []string{raw}}).Normalize(parser, "", false)
			if err != nil || !reflect.DeepEqual(included.Tags, snapshot.Tags) || !included.Matches(snapshot.Tags) {
				t.Fatalf("published=%U included=%+v err=%v", []rune(snapshot.Tags[0]), included, err)
			}
			excluded, err := (content.TagFilter{ExcludeTags: []string{raw}}).Normalize(parser, "", false)
			if err != nil || excluded.Matches(snapshot.Tags) {
				t.Fatalf("exclusion %+v %v", excluded, err)
			}
			legacy, err := (content.TagFilter{}).Normalize(parser, raw, true)
			if err != nil || !reflect.DeepEqual(legacy, included) {
				t.Fatalf("legacy %+v %v", legacy, err)
			}
		})
	}
	composed, _ := (content.TagFilter{Tags: []string{"ǰ"}}).Normalize(parser, "", false)
	decomposed, _ := (content.TagFilter{Tags: []string{"j\u030c", "ǰ"}, Match: "any"}).Normalize(parser, "", false)
	if !reflect.DeepEqual(composed, decomposed) {
		t.Fatalf("cursor predicates differ: %+v %+v", composed, decomposed)
	}
}
