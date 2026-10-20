package text

import (
	"strings"
	"testing"
)

func TestSearchPlainText(t *testing.T) {
	out, err := (Parser{}).Plain("# 中文 **Orbit**\n\n[label](https://hidden-target.invalid) &amp; `code`\n\n```go\nvar value = 1\n```\n\n<script>excluded script</script>\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"中文 Orbit", "label & code", "var value = 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	for _, bad := range []string{"hidden-target", "script", "<", ">", "```"} {
		if strings.Contains(out, bad) {
			t.Fatalf("included %q", bad)
		}
	}
}
