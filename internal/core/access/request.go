package access

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

// JSONHash ignores object key order and insignificant whitespace, while keeping
// a legacy byte digest so pre-MCP idempotency records remain replayable.
func JSONHash(raw []byte) string {
	legacy := identity.Digest(string(raw))
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return legacy
	}
	if decoder.Decode(new(any)) != io.EOF {
		return legacy
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return legacy
	}
	return "json1:" + identity.Digest(string(canonical)) + ":" + legacy
}
func SameRequestHash(previous, current string) bool {
	if previous == current {
		return true
	}
	parts := func(s string) (string, string) {
		p := strings.Split(s, ":")
		if len(p) == 3 && p[0] == "json1" && len(p[1]) == 64 && len(p[2]) == 64 {
			return p[1], p[2]
		}
		return "", s
	}
	a, old := parts(previous)
	b, now := parts(current)
	return a != "" && b != "" && a == b || old == now
}
