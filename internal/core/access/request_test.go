package access

import (
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func TestJSONIdempotencyAcrossEnvelopes(t *testing.T) {
	original := []byte(`{"head":null,"version":9007199254740993,"body":"text"}`)
	reordered := []byte(" {\n\"body\":\"text\",\"version\":9007199254740993,\"head\":null }")
	if !SameRequestHash(JSONHash(original), JSONHash(reordered)) {
		t.Fatal("JSON representation affected replay")
	}
	if !SameRequestHash(identity.Digest(string(original)), JSONHash(original)) {
		t.Fatal("legacy replay lost")
	}
	for _, different := range []string{`{"head":null,"version":9007199254740992,"body":"text"}`, `{"head":null,"version":9007199254740993,"body":"changed"}`} {
		if SameRequestHash(JSONHash(original), JSONHash([]byte(different))) {
			t.Fatal("different payload replayed")
		}
	}
}
