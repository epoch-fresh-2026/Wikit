package proposals

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
)

// Edit replaces base lines [Start,End); Text preserves original line endings.
type Edit struct {
	Start, End int
	Text       string
}
type Lines interface {
	Edits(context.Context, string, string) ([]Edit, error)
}

func lineSlice(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
func overlap(a, b Edit) bool {
	if a.Start == a.End && b.Start == b.End {
		return a.Start == b.Start
	}
	if a.Start == a.End {
		return a.Start > b.Start && a.Start < b.End
	}
	if b.Start == b.End {
		return b.Start > a.Start && b.Start < a.End
	}
	return a.Start < b.End && b.Start < a.End
}
func apply(lines []string, start, end int, edits []Edit) string {
	var out strings.Builder
	pos := start
	for _, e := range edits {
		out.WriteString(strings.Join(lines[pos:e.Start], ""))
		out.WriteString(e.Text)
		pos = e.End
	}
	out.WriteString(strings.Join(lines[pos:end], ""))
	return out.String()
}
func mergeLines(ctx context.Context, engine Lines, base, ours, theirs string) (string, bool, error) {
	if ours == theirs || theirs == base {
		return ours, false, nil
	}
	if ours == base {
		return theirs, false, nil
	}
	a, err := engine.Edits(ctx, base, ours)
	if err != nil {
		return "", false, err
	}
	b, err := engine.Edits(ctx, base, theirs)
	if err != nil {
		return "", false, err
	}
	// A cluster includes every cross-side overlapping edit, transitively.
	type marked struct {
		edit Edit
		side int
	}
	all := make([]marked, 0, len(a)+len(b))
	for _, e := range a {
		all = append(all, marked{e, 0})
	}
	for _, e := range b {
		all = append(all, marked{e, 1})
	}
	slices.SortStableFunc(all, func(a, b marked) int {
		if a.edit.Start != b.edit.Start {
			return a.edit.Start - b.edit.Start
		}
		return a.edit.End - b.edit.End
	})
	lines := lineSlice(base)
	pos := 0
	var out strings.Builder
	for i := 0; i < len(all); {
		end := i + 1
		extent := all[i].edit.End
		last := [2]int{-1, -1}
		last[all[i].side] = i
		for end < len(all) {
			// Each side's edits are ordered and nonoverlapping. Only its
			// latest edit can overlap the next edit from the other side.
			other := last[1-all[end].side]
			if other < 0 || !overlap(all[other].edit, all[end].edit) {
				break
			}
			last[all[end].side] = end
			extent = max(extent, all[end].edit.End)
			end++
		}
		start := all[i].edit.Start
		out.WriteString(strings.Join(lines[pos:start], ""))
		left, right := []Edit{}, []Edit{}
		for _, m := range all[i:end] {
			if m.side == 0 {
				left = append(left, m.edit)
			} else {
				right = append(right, m.edit)
			}
		}
		switch {
		case len(left) == 0:
			out.WriteString(apply(lines, start, extent, right))
		case len(right) == 0:
			out.WriteString(apply(lines, start, extent, left))
		default:
			x, y := apply(lines, start, extent, left), apply(lines, start, extent, right)
			if x != y {
				return "", true, nil
			}
			out.WriteString(x)
		}
		pos = extent
		i = end
	}
	out.WriteString(strings.Join(lines[pos:], ""))
	return out.String(), false, ctx.Err()
}
func value(s string) *string { return &s }
func emptySnapshot(kind string) content.Snapshot {
	s := content.Snapshot{Type: kind, Tags: []string{}}
	if kind == "document" {
		s.Markdown = value("")
	} else {
		s.Description = value("")
	}
	return s
}
func mergeSnapshots(ctx context.Context, engine Lines, b, h, p content.Snapshot) (content.Snapshot, error) {
	out := p
	conflicts := []fault.Conflict{}
	scalar := func(path string, base, ours, theirs *string) *string {
		if same(ours, theirs) || same(theirs, base) {
			return ours
		}
		if same(ours, base) {
			return theirs
		}
		conflicts = append(conflicts, fault.Conflict{Path: path, Base: base, Ours: ours, Theirs: theirs})
		return theirs
	}
	out.Title = *scalar("/title", value(b.Title), value(h.Title), value(p.Title))
	if p.Type == "document" {
		merged, conflict, err := mergeLines(ctx, engine, *b.Markdown, *h.Markdown, *p.Markdown)
		if err != nil {
			return out, err
		}
		if conflict {
			conflicts = append(conflicts, fault.Conflict{Path: "/markdown", Base: b.Markdown, Ours: h.Markdown, Theirs: p.Markdown})
		} else {
			out.Markdown = &merged
		}
	} else {
		out.Description = scalar("/description", b.Description, h.Description, p.Description)
		out.BlobID = scalar("/blob_id", b.BlobID, h.BlobID, p.BlobID)
	}
	keys := map[string]bool{}
	for _, tags := range [][]string{b.Tags, h.Tags, p.Tags} {
		for _, tag := range tags {
			keys[tag] = true
		}
	}
	out.Tags = []string{}
	for tag := range keys {
		base, ours, theirs := slices.Contains(b.Tags, tag), slices.Contains(h.Tags, tag), slices.Contains(p.Tags, tag)
		keep := ours
		if ours == base {
			keep = theirs
		}
		if keep {
			out.Tags = append(out.Tags, tag)
		}
	}
	slices.Sort(out.Tags)
	if len(conflicts) > 0 {
		return out, &fault.Conflicts{Fields: conflicts}
	}
	return out, nil
}

type Field struct {
	Path        string  `json:"path"`
	Before      *string `json:"before"`
	After       *string `json:"after"`
	UnifiedDiff *string `json:"unified_diff"`
}
type Diff struct {
	BaseRevisionID     *string `json:"base_revision_id"`
	HeadRevisionID     *string `json:"head_revision_id"`
	ProposalRevisionID string  `json:"proposal_revision_id"`
	Stale              bool    `json:"stale"`
	Fields             []Field `json:"fields"`
}

func fields(s content.Snapshot) (map[string]*string, error) {
	tags, err := json.Marshal(s.Tags)
	if err != nil {
		return nil, err
	}
	return map[string]*string{"/title": value(s.Title), "/tags": value(string(tags)), "/markdown": s.Markdown, "/description": s.Description, "/blob_id": s.BlobID}, nil
}

// unified emits a complete single-hunk line diff, including missing-newline markers.
func unified(ctx context.Context, engine Lines, b, p string) (string, error) {
	edits, err := engine.Edits(ctx, b, p)
	if err != nil {
		return "", err
	}
	before, after := lineSlice(b), lineSlice(p)
	var out strings.Builder
	out.WriteString("--- base\n+++ proposal\n")
	// Header line ranges use unified-diff's zero start for an empty file.
	bs, ps := 1, 1
	if len(before) == 0 {
		bs = 0
	}
	if len(after) == 0 {
		ps = 0
	}
	out.WriteString(formatRange(bs, len(before), ps, len(after)))
	line := func(prefix string, text string) {
		for _, l := range lineSlice(text) {
			out.WriteString(prefix)
			out.WriteString(l)
			if !strings.HasSuffix(l, "\n") {
				out.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	pos := 0
	for _, e := range edits {
		line(" ", strings.Join(before[pos:e.Start], ""))
		line("-", strings.Join(before[e.Start:e.End], ""))
		line("+", e.Text)
		pos = e.End
	}
	line(" ", strings.Join(before[pos:], ""))
	return out.String(), nil
}
