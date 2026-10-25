package search

import (
	"reflect"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
)

func TestFusionPromotesAgreementAndKeepsStableTies(t *testing.T) {
	h := func(id string) Hit { return Hit{PageID: id, RevisionID: "current"} }
	out := fuseRanks([][]Hit{{h("one-off"), h("shared")}, {h("other"), h("shared")}})
	ids := []string{}
	for _, c := range out {
		ids = append(ids, c.ID)
	}
	if !reflect.DeepEqual(ids, []string{"shared", "one-off", "other"}) {
		t.Fatal(ids)
	}
}

func TestFusionDuplicateVotesAndRevisionsAreIsolated(t *testing.T) {
	a := Hit{PageID: "A", RevisionID: "old"}
	b := Hit{PageID: "B", RevisionID: "current"}
	newA := Hit{PageID: "A", RevisionID: "new"}
	out := fuseRanks([][]Hit{{a, a, a, b}, {newA, b}})
	if len(out) != 3 || out[0].ID != "B" || out[1].Revision != "old" || out[2].Revision != "new" {
		t.Fatal(out)
	}
	if got := fuseRanks(nil); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestNamedSubjectDiversityPreservesRanksWithinPasses(t *testing.T) {
	makeSource := func(id, title string) Source {
		return Source{Page: content.Page{ID: id}, Snapshot: content.Snapshot{Title: title}}
	}
	in := []Source{makeSource("1", "枫树"), makeSource("2", "枫树"), makeSource("3", "枫树"), makeSource("4", "桦树"), makeSource("5", "桦树")}
	out := diversifySubjects(in)
	ids := []string{}
	for _, src := range out {
		ids = append(ids, src.Page.ID)
	}
	if !reflect.DeepEqual(ids, []string{"1", "4", "2", "3", "5"}) || in[1].Page.ID != "2" {
		t.Fatal(ids, in)
	}
}
