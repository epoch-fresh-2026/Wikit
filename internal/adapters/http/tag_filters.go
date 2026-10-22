package http

import (
	"net/url"
	"slices"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
)

// Only the two list-valued parameters may repeat. Semantic validation is Core's job.
func tagQuery(values url.Values, search bool) (content.TagFilter, error) {
	allowed := []string{"tag", "tags", "tag_match", "exclude_tags", "type", "limit", "cursor"}
	if search {
		allowed = append(allowed, "q")
	}
	for key, items := range values {
		if !slices.Contains(allowed, key) || (key != "tags" && key != "exclude_tags" && len(items) != 1) {
			return content.TagFilter{}, fault.BadRequest
		}
	}
	return content.TagFilter{Tags: values["tags"], Match: values.Get("tag_match"), MatchSet: values.Has("tag_match"), ExcludeTags: values["exclude_tags"]}, nil
}
