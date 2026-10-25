package search

import (
	"fmt"
	"slices"
	"strings"
)

type Passage struct {
	Text        string   `json:"text"`
	SectionPath []string `json:"section_path"`
	SectionID   *string  `json:"section_id"`
	StartLine   int      `json:"start_line"`
	EndLine     int      `json:"end_line"`
	Truncated   bool     `json:"truncated"`
	CitationURL string   `json:"citation_url"`
}

func bounded(s string, n int) string { r := []rune(s); return string(r[:min(len(r), n)]) }

// Evidence selection happens only after the source's final authorization check.
func passages(source Source, query string, text Text) ([]Passage, error) {
	return selectPassages(source, query, text, false)
}

func ragPassages(source Source, query string, text Text) ([]Passage, error) {
	return selectPassages(source, query, text, true)
}

func selectPassages(source Source, query string, text Text, deduplicate bool) ([]Passage, error) {
	all, err := passageCandidates(source, query, text, deduplicate)
	if err != nil {
		return nil, err
	}
	return all[:min(3, len(all))], nil
}

func passageCandidates(source Source, query string, text Text, deduplicate bool) ([]Passage, error) {
	markdown := ""
	if source.Snapshot.Markdown != nil {
		markdown = *source.Snapshot.Markdown
	} else if source.Snapshot.Description != nil {
		markdown = *source.Snapshot.Description
	}
	blocks, err := text.Blocks(markdown)
	if err != nil {
		return nil, err
	}
	terms := strings.FieldsFunc(text.Fold(text.NFC(query)), func(r rune) bool { return r == '"' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	terms = slices.Compact(slices.Sorted(slices.Values(terms)))
	score := func(value string) int {
		value = text.Fold(text.NFC(value))
		n := 0
		for _, term := range terms {
			if strings.Contains(value, term) {
				n++
			}
		}
		return n
	}
	type candidate struct {
		passage Passage
		score   int
	}
	candidates := []candidate{}
	type duplicateKey struct{ section, text string }
	seen := map[duplicateKey]bool{}
	for _, block := range blocks {
		runes := []rune(block.Text)
		// Exact full-block duplicates only. Equal truncated windows or identical
		// words in different sections can still carry independent meaning.
		if deduplicate && len(runes) <= 600 {
			section := ""
			if block.SectionID != nil {
				section = *block.SectionID
			}
			key := duplicateKey{section, block.Text}
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		best, bestScore := "", -1
		// Overlapping windows avoid cutting a hit at the start of a long block.
		for start := 0; start < len(runes); start += 100 {
			end := min(start+600, len(runes))
			window := string(runes[start:end])
			n := score(window)
			if n > bestScore {
				best, bestScore = window, n
			}
			if end == len(runes) {
				break
			}
		}
		contextScore := score(strings.Join(block.SectionPath, " "))
		if bestScore <= 0 && contextScore == 0 {
			continue
		}
		path := []string{}
		for _, h := range block.SectionPath {
			path = append(path, bounded(h, 300))
		}
		citation := fmt.Sprintf("/pages/%s/revisions/%s", source.Page.ID, *source.Page.HeadRevisionID)
		if block.SectionID != nil {
			citation += "#" + *block.SectionID
		}
		p := Passage{best, path, block.SectionID, block.StartLine, block.EndLine, len(runes) > 600, citation}
		candidates = append(candidates, candidate{p, max(0, bestScore)*10 + contextScore})
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int { return b.score - a.score })
	out := []Passage{}
	for _, c := range candidates {
		out = append(out, c.passage)
	}
	return out, nil
}

func requirementPassages(source Source, plan questionSpec, text Text) ([]Passage, bool, error) {
	if plan.boundClauses {
		plan.Requirements = subjectRequirements(plan, titleAlias(source.Snapshot.Title))
		if len(plan.Requirements) == 0 {
			return []Passage{}, false, nil
		}
		plan.focus = requirementFocus(plan.Requirements[0])
	}
	if len(plan.Requirements) < 2 {
		ps, err := ragPassages(source, plan.focus, text)
		return ps, false, err
	}
	branches := [][]Passage{}
	for _, r := range plan.Requirements {
		ps, err := passageCandidates(source, requirementFocus(r), text, true)
		if err != nil {
			return nil, false, err
		}
		slices.SortStableFunc(ps, func(a, b Passage) int {
			av, bv := attributePresent(a, r.Attribute), attributePresent(b, r.Attribute)
			if av == bv {
				return 0
			}
			if av {
				return -1
			}
			return 1
		})
		branches = append(branches, ps)
	}
	out := []Passage{}
	for round := 0; round < 3; round++ {
		for _, ps := range branches {
			if round >= len(ps) {
				continue
			}
			p := ps[round]
			if slices.ContainsFunc(out, func(x Passage) bool {
				return x.StartLine == p.StartLine && x.EndLine == p.EndLine && x.Text == p.Text && x.CitationURL == p.CitationURL
			}) {
				continue
			}
			if len(out) < 3 {
				out = append(out, p)
			}
		}
	}
	truncated := false
	for i, r := range plan.Requirements {
		available := slices.ContainsFunc(branches[i], func(p Passage) bool { return attributePresent(p, r.Attribute) })
		selected := slices.ContainsFunc(out, func(p Passage) bool { return attributePresent(p, r.Attribute) })
		if available && !selected {
			truncated = true
		}
	}
	return out, truncated, nil
}
