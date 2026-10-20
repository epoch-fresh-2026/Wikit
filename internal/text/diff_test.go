package text

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestLineEditsPreserveBytes(t *testing.T) {
	var unique strings.Builder
	for i := 0; i < 60000; i++ {
		fmt.Fprintf(&unique, "line %d\n", i)
	}
	large := unique.String()
	for _, pair := range [][2]string{{"a\nb\n", "a\nx\nb\n"}, {"a\nb", "a\nB"}, {"a\r\nb\r\n", "a\r\nB\r\n"}, {"", "new"}, {"old", ""}, {large, large + "last line"}} {
		edits, err := (Differ{}).Edits(context.Background(), pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.SplitAfter(pair[0], "\n")
		if pair[0] == "" {
			lines = nil
		} else if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		var out strings.Builder
		pos := 0
		for _, e := range edits {
			out.WriteString(strings.Join(lines[pos:e.Start], ""))
			out.WriteString(e.Text)
			pos = e.End
		}
		out.WriteString(strings.Join(lines[pos:], ""))
		if out.String() != pair[1] {
			t.Fatal("line edits changed bytes")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Differ{}).Edits(ctx, "a", "b"); err != context.Canceled {
		t.Fatal("ignored cancellation")
	}
}
