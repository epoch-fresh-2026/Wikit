package search

import "slices"

// fuseRanks combines only identities and ranks. Content and authorization must
// be read again in the final transaction. Revisions never vote for each other.
func fuseRanks(branches [][]Hit) []Candidate {
	type key struct{ page, revision string }
	type ranked struct {
		candidate Candidate
		score     float64
	}
	positions := map[key]int{}
	rows := []ranked{}
	for _, branch := range branches {
		seen := map[key]bool{}
		for rank, h := range branch {
			k := key{h.PageID, h.RevisionID}
			if seen[k] {
				continue
			}
			seen[k] = true
			i, ok := positions[k]
			if !ok {
				i = len(rows)
				positions[k] = i
				rows = append(rows, ranked{candidate: Candidate{ID: h.PageID, Revision: h.RevisionID}})
			}
			rows[i].score += 1 / float64(60+rank+1)
		}
	}
	slices.SortStableFunc(rows, func(a, b ranked) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}
		return 0
	})
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.candidate)
	}
	return out
}

// diversifySubjects is called only for authorized named sources. Preserve rank
// within each pass, reserving the first pass for one page per requested subject.
func diversifySubjects(sources []Source) []Source {
	seen := map[string]bool{}
	first, rest := []Source{}, []Source{}
	for _, src := range sources {
		name := titleAlias(src.Snapshot.Title)
		if seen[name] {
			rest = append(rest, src)
		} else {
			first = append(first, src)
			seen[name] = true
		}
	}
	return append(first, rest...)
}
