package text

import (
	"encoding/json"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"slices"
	"strings"
	"testing"
)

func stringPtr(s string) *string { return &s }
func TestSnapshotNormalizationAndMarkdown(t *testing.T) {
	p := Parser{}
	in := content.Snapshot{Type: "document", Title: "  Cafe\u0301  ", Tags: []string{" Go ", "go", "Straße", "STRASSE", "e\u0301", "é"}, Markdown: stringPtr("example `![ignored](/api/v1/pages/nope/content)`")}
	out, refs, err := content.Normalize(p, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Café" || !slices.Equal(out.Tags, []string{"go", "strasse", "é"}) || len(refs) != 0 {
		t.Fatal("incorrect canonical snapshot", out.Title, out.Tags, refs)
	}
	for _, markdown := range []string{"[x](javascript:alert%281%29)", "[x](javascript&#58;alert)", "[x](data:text/html,x)", "[x](/api/v1/pages/2/content)"} {
		in.Markdown = &markdown
		if _, _, err := content.Normalize(p, in); err != fault.Invalid {
			t.Fatalf("unsafe/unfixed link accepted: %s", markdown)
		}
	}
	link := "/api/v1/pages/2/revisions/00000000-0000-4000-8000-000000000002/content"
	for _, variant := range []string{
		strings.Replace(link, "/v1/", "/v1/./", 1),
		strings.Replace(link, "/v1/", "/v1//", 1),
		strings.Replace(link, "/v1/", "/v1/unused/../", 1),
		strings.Replace(link, "/v1/", "/v1/%2e/", 1),
		strings.Replace(link, "/v1/", "/v1/%5c", 1),
	} {
		in.Markdown = stringPtr("[x](" + variant + ")")
		if _, _, err := content.Normalize(p, in); err != fault.Invalid {
			t.Fatalf("noncanonical internal media reference accepted: %s", variant)
		}
	}
	in.Markdown = stringPtr("![x][ref]\n\n[ref]: " + link + "\n\n[unused]: /api/v1/pages/invalid/content")
	_, refs, err = content.Normalize(p, in)
	if err != nil || len(refs) != 1 || refs[0].RevisionID != "00000000-0000-4000-8000-000000000002" {
		t.Fatal("reference-style Markdown", refs, err)
	}
	for _, suffix := range []string{"?x=1", "#fragment"} {
		in.Markdown = stringPtr("![x](" + link + suffix + ")")
		if _, _, err := content.Normalize(p, in); err != fault.Invalid {
			t.Fatal("noncanonical media reference accepted")
		}
	}
}
func TestSnapshotVariantsAndLimits(t *testing.T) {
	p := Parser{}
	for _, in := range []content.Snapshot{
		{Type: "document", Title: " ", Tags: []string{}, Markdown: stringPtr("")},
		{Type: "document", Title: "Title", Tags: nil, Markdown: stringPtr("")},
		{Type: "document", Title: "Title", Tags: []string{}, Markdown: stringPtr(""), Description: stringPtr("unexpected")},
		{Type: "media", Title: "Title", Tags: []string{}, Description: stringPtr(""), BlobID: stringPtr("bad")},
	} {
		if _, _, err := content.Normalize(p, in); err != fault.Invalid {
			t.Fatal("invalid snapshot accepted")
		}
	}
	var snapshot content.Snapshot
	if err := json.Unmarshal([]byte(`{"type":"document","title":"Title","tags":[],"markdown":"","blob_id":null}`), &snapshot); err == nil {
		t.Fatal("null field bypassed closed snapshot union")
	}
}
