package access

import "testing"

func TestSubjectIDMatchesKind(t *testing.T) {
	const group = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	for _, tc := range []struct {
		kind, id string
		valid    bool
	}{
		{"user", "1", true}, {"user", "9223372036854775807", true},
		{"user", "01", false}, {"user", group, false},
		{"group", group, true}, {"group", "1", false},
		{"agent", "1", false}, {"", group, false},
	} {
		if got := validSubjectID(tc.kind, tc.id); got != tc.valid {
			t.Errorf("subject %s/%s: got %v", tc.kind, tc.id, got)
		}
	}
}
