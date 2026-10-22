package content_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
)

type testTagText struct{}

func (testTagText) NFC(s string) string  { return s }
func (testTagText) Fold(s string) string { return strings.ToLower(s) }

func TestTagFilterSemanticsAndCanonicalization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter content.TagFilter
		tags   []string
		want   bool
	}{
		{"all", content.TagFilter{Tags: []string{"a", "b"}}, []string{"a", "b"}, true},
		{"all missing", content.TagFilter{Tags: []string{"a", "b"}}, []string{"a"}, false},
		{"any", content.TagFilter{Tags: []string{"a", "b"}, Match: "any"}, []string{"b"}, true},
		{"any missing", content.TagFilter{Tags: []string{"a", "b"}, Match: "any"}, nil, false},
		{"only exclude", content.TagFilter{ExcludeTags: []string{"x"}}, []string{"a"}, true},
		{"exclude wins", content.TagFilter{Tags: []string{"a", "b"}, Match: "any", ExcludeTags: []string{"b"}}, []string{"a", "b"}, false},
		{"overlap allows other", content.TagFilter{Tags: []string{"a", "b"}, Match: "any", ExcludeTags: []string{"b"}}, []string{"a"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.filter.Normalize(testTagText{}, "", false)
			if err != nil || f.Matches(tc.tags) != tc.want {
				t.Fatalf("filter=%+v err=%v", f, err)
			}
		})
	}
	a, err := (content.TagFilter{Tags: []string{" B ", "A", "b"}}).Normalize(testTagText{}, "", false)
	if err != nil || !reflect.DeepEqual(a.Tags, []string{"a", "b"}) {
		t.Fatalf("canonical %+v %v", a, err)
	}
	b, err := (content.TagFilter{Tags: []string{"B"}, Match: "any"}).Normalize(testTagText{}, "", false)
	c, _ := (content.TagFilter{}).Normalize(testTagText{}, " b ", true)
	if err != nil || !reflect.DeepEqual(b, c) {
		t.Fatalf("legacy differs: %+v %+v", b, c)
	}
	for _, f := range []content.TagFilter{
		{Tags: []string{" "}}, {Tags: []string{"\x00"}}, {Tags: []string{string([]byte{255})}},
		{Tags: []string{strings.Repeat("中", 65)}}, {Match: "or"}, {MatchSet: true},
		{Tags: make([]string, 31)}, {Tags: []string{"a"}, ExcludeTags: make([]string, 30)},
	} {
		if _, err := f.Normalize(testTagText{}, "", false); err == nil {
			t.Fatalf("accepted %+v", f)
		}
	}
	if _, err := (content.TagFilter{Tags: []string{}}).Normalize(testTagText{}, "", true); err == nil {
		t.Fatal("accepted legacy/new conflict")
	}
}
