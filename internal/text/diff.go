package text

import (
	"context"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/sergi/go-diff/diffmatchpatch"
)

type Differ struct{}

func (Differ) Edits(ctx context.Context, base, target string) ([]proposals.Edit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToRunes(base, target)
	diffs := dmp.DiffMainRunes(a, b, false)
	out := []proposals.Edit{}
	pos := 0
	var pending *proposals.Edit
	flush := func() {
		if pending != nil {
			out = append(out, *pending)
			pending = nil
		}
	}
	hydrated := dmp.DiffCharsToLines(diffs, lines)
	for i, d := range diffs {
		n := utf8.RuneCountInString(d.Text)
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			flush()
			pos += n
		case diffmatchpatch.DiffDelete:
			if pending == nil {
				pending = &proposals.Edit{Start: pos, End: pos}
			}
			pos += n
			pending.End = pos
		case diffmatchpatch.DiffInsert:
			if pending == nil {
				pending = &proposals.Edit{Start: pos, End: pos}
			}
			pending.Text += hydrated[i].Text
		}
	}
	flush()
	return out, ctx.Err()
}
