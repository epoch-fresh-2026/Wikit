// Package content owns immutable document/media snapshot values and validation.
package content

import (
	"bytes"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type Snapshot struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	Markdown    *string  `json:"markdown,omitempty"`
	Description *string  `json:"description,omitempty"`
	BlobID      *string  `json:"blob_id,omitempty"`
}

// UnmarshalJSON preserves the closed document/media union even when an extra
// field is null (a pointer alone cannot distinguish null from an absent field).
func (s *Snapshot) UnmarshalJSON(raw []byte) error {
	type wire Snapshot
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	required := []string{"type", "title", "tags"}
	switch value.Type {
	case "document":
		required = append(required, "markdown")
	case "media":
		required = append(required, "description", "blob_id")
	default:
		return fault.Invalid
	}
	if len(fields) != len(required) {
		return fault.Invalid
	}
	for _, key := range required {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fault.Invalid
		}
	}
	*s = Snapshot(value)
	return nil
}

type MediaRef struct{ PageID, RevisionID string }

// Text supplies Unicode and CommonMark primitives, never business rules.
type Text interface {
	NFC(string) string
	Fold(string) string
	Links(string) ([]string, error)
}

func TagKey(text Text, value string) string {
	return text.NFC(text.Fold(text.NFC(strings.TrimSpace(value))))
}
func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func Normalize(text Text, in Snapshot) (Snapshot, []MediaRef, error) {
	if !validText(in.Title) || utf8.RuneCountInString(in.Title) > 300 || in.Tags == nil || len(in.Tags) > 30 {
		return Snapshot{}, nil, fault.Invalid
	}
	in.Title = text.NFC(strings.TrimSpace(in.Title))
	if in.Title == "" {
		return Snapshot{}, nil, fault.Invalid
	}
	seen := map[string]bool{}
	tags := []string{}
	for _, raw := range in.Tags {
		if !validText(raw) {
			return Snapshot{}, nil, fault.Invalid
		}
		key := TagKey(text, raw)
		if key == "" || utf8.RuneCountInString(key) > 64 {
			return Snapshot{}, nil, fault.Invalid
		}
		if !seen[key] {
			tags = append(tags, key)
			seen[key] = true
		}
	}
	slices.Sort(tags)
	in.Tags = tags
	var markdown string
	switch in.Type {
	case "document":
		if in.Markdown == nil || in.Description != nil || in.BlobID != nil || !validText(*in.Markdown) || utf8.RuneCountInString(*in.Markdown) > 1048576 {
			return Snapshot{}, nil, fault.Invalid
		}
		markdown = *in.Markdown
	case "media":
		if in.Markdown != nil || in.Description == nil || in.BlobID == nil || !identity.ValidID(*in.BlobID) || !validText(*in.Description) || utf8.RuneCountInString(*in.Description) > 65536 {
			return Snapshot{}, nil, fault.Invalid
		}
		id := strings.ToLower(*in.BlobID)
		in.BlobID = &id
		markdown = *in.Description
	default:
		return Snapshot{}, nil, fault.Invalid
	}
	links, err := text.Links(markdown)
	if err != nil {
		return Snapshot{}, nil, fault.Invalid
	}
	refs := []MediaRef{}
	targets := map[string]bool{}
	for _, destination := range links {
		u, err := url.Parse(destination)
		if err != nil || u.User != nil {
			return Snapshot{}, nil, fault.Invalid
		}
		if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto" {
			return Snapshot{}, nil, fault.Invalid
		}
		if strings.HasPrefix(u.Path, "/api/v1/pages/") {
			parts := strings.Split(u.Path, "/")
			if len(parts) != 8 || parts[5] != "revisions" || parts[7] != "content" || !identity.ValidID(parts[4]) || !identity.ValidID(parts[6]) || u.Host != "" || u.Scheme != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
				return Snapshot{}, nil, fault.Invalid
			}
			ref := MediaRef{strings.ToLower(parts[4]), strings.ToLower(parts[6])}
			key := ref.PageID + ":" + ref.RevisionID
			if !targets[key] {
				targets[key] = true
				refs = append(refs, ref)
			}
			if len(refs) > 1000 {
				return Snapshot{}, nil, fault.Invalid
			}
		}
	}
	return in, refs, nil
}
