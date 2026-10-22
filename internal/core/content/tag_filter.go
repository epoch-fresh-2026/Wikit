package content

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
)

// TagFilter is a flat inclusion predicate followed by an exclusion predicate.
// Normalize must be called before evaluating or binding it to a cursor.
type TagFilter struct {
	Tags        []string
	Match       string
	ExcludeTags []string
	MatchSet    bool `json:"-"`
}

func (f TagFilter) Normalize(text interface {
	NFC(string) string
	Fold(string) string
}, legacy string, legacySet bool) (TagFilter, error) {
	if (legacySet || legacy != "") && f.Tags != nil {
		return TagFilter{}, fault.BadRequest
	}
	if f.Match == "" {
		if f.MatchSet {
			return TagFilter{}, fault.BadRequest
		}
		f.Match = "all"
	}
	if f.Match != "all" && f.Match != "any" {
		return TagFilter{}, fault.BadRequest
	}
	if legacy != "" {
		f.Tags = []string{legacy}
	}
	if len(f.Tags)+len(f.ExcludeTags) > 30 {
		return TagFilter{}, fault.BadRequest
	}
	normalize := func(values []string) ([]string, error) {
		out := make([]string, 0, len(values))
		for _, v := range values {
			if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
				return nil, fault.BadRequest
			}
			v = text.Fold(text.NFC(strings.TrimSpace(v)))
			if v == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > 64 {
				return nil, fault.BadRequest
			}
			out = append(out, v)
		}
		slices.Sort(out)
		return slices.Compact(out), nil
	}
	var err error
	if f.Tags, err = normalize(f.Tags); err != nil {
		return TagFilter{}, err
	}
	if f.ExcludeTags, err = normalize(f.ExcludeTags); err != nil {
		return TagFilter{}, err
	}
	if len(f.Tags) <= 1 {
		f.Match = "all"
	}
	f.MatchSet = false
	return f, nil
}

func (f TagFilter) Matches(tags []string) bool {
	for _, tag := range f.ExcludeTags {
		if slices.Contains(tags, tag) {
			return false
		}
	}
	if len(f.Tags) == 0 {
		return true
	}
	for _, tag := range f.Tags {
		has := slices.Contains(tags, tag)
		if f.Match == "any" && has {
			return true
		}
		if f.Match == "all" && !has {
			return false
		}
	}
	return f.Match == "all"
}
