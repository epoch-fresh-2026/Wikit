package search

import (
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"slices"
	"testing"
)

func TestClauseBindingsAndSharedGroups(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"赤槐的获得方式和灰栎的攻击间隔分别是什么？", []string{"赤槐/获得方式", "灰栎/攻击间隔"}},
		{"赤槐和灰栎的获得方式与攻击间隔分别是什么？", []string{"赤槐/获得方式", "赤槐/攻击间隔", "灰栎/获得方式", "灰栎/攻击间隔"}},
		{"赤槐与蓝杉的生命上限和攻击、灰栎的获得方式分别是什么？", []string{"赤槐/生命上限", "赤槐/攻击", "蓝杉/生命上限", "蓝杉/攻击", "灰栎/获得方式"}},
		{"请查赤槐的阻挡数；灰栎的所属势力、获得方式都是什么？", []string{"赤槐/阻挡数", "灰栎/所属势力", "灰栎/获得方式"}},
		{"赤槐精英一满级的攻击与灰栎精英二满级的生命上限是多少？", []string{"赤槐/攻击/精英1", "灰栎/生命上限/精英2"}},
		{"赤槐精英一的生命上限与赤槐精英二的生命上限是多少？", []string{"赤槐/生命上限/精英1", "赤槐/生命上限/精英2"}},
		{"赤槐的攻击和未知树的获得方式分别是什么？", []string{"赤槐/攻击", "未知树/获得方式"}},
		{"和风的获得方式以及说明的攻击间隔是多少？", []string{"和风/获得方式", "说明/攻击间隔"}},
	} {
		t.Run(tc.q, func(t *testing.T) {
			p := parseQuestion(tc.q, []string{"赤槐", "灰栎", "蓝杉", "和风", "说明"})
			got := []string{}
			for _, b := range p.Bindings {
				k := b.Subject + "/" + b.Attribute
				if b.Stage != nil {
					k += "/" + *b.Stage
				}
				got = append(got, k)
			}
			slices.Sort(got)
			slices.Sort(tc.want)
			if !slices.Equal(got, tc.want) || len(p.Constraints) > 0 {
				t.Fatalf("bindings=%v plan=%+v", got, p)
			}
		})
	}
}

func TestClauseBindingKeepsUnresolvedConditions(t *testing.T) {
	for _, q := range []string{
		"精英二时赤槐的生命上限与灰栎的攻击是多少？",
		"赤槐的生命上限与灰栎受到攻击提升30%后的攻击是多少？",
		"赤槐的攻击与灰栎的生日是什么？",
	} {
		p := parseQuestion(q, []string{"赤槐", "灰栎"})
		if len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
}

func TestBoundCoverageCannotBorrowAnotherSubjectAttribute(t *testing.T) {
	p := parseQuestion("赤槐的获得方式和灰栎的攻击间隔分别是什么？", []string{"赤槐", "灰栎"})
	items := []Hit{{PageID: "1", RevisionID: "r1", Title: "赤槐"}, {PageID: "2", RevisionID: "r2", Title: "灰栎"}}
	evidence := []Evidence{{PageID: "1", RevisionID: "r1", CitationID: "E1", Passage: Passage{Text: "获得方式 任务奖励"}}, {PageID: "2", RevisionID: "r2", CitationID: "E2", Passage: Passage{Text: "攻击间隔 2s"}}}
	c := assessCoverage(p, items, evidence, nil, false)
	if c.Status != "complete" || len(c.Requirements) != 2 {
		t.Fatal(c)
	}
	evidence[0].PageID = "2"
	evidence[0].RevisionID = "r2"
	c = assessCoverage(p, items, evidence, nil, false)
	if c.Status != "partial" || c.Requirements[0].Status != "attribute_missing" {
		t.Fatal(c)
	}
	c = assessCoverage(p, items[1:], evidence, nil, false)
	if c.Status != "partial" || !slices.Equal(c.MissingSubjects, []string{"赤槐"}) {
		t.Fatal(c)
	}
}

func TestBoundEvidenceUsesOnlyApplicableRequirements(t *testing.T) {
	p := parseQuestion("赤槐的获得方式和灰栎的攻击间隔分别是什么？", []string{"赤槐", "灰栎"})
	rev := "r"
	src := Source{Page: content.Page{ID: "1", HeadRevisionID: &rev}, Snapshot: content.Snapshot{Title: "赤槐"}}
	ps, truncated, err := requirementPassages(src, p, blockText{blocks: []TextBlock{{Text: "攻击间隔 2s", StartLine: 1, EndLine: 1}, {Text: "获得方式 任务奖励", StartLine: 2, EndLine: 2}}})
	if err != nil || truncated || len(ps) != 1 || ps[0].StartLine != 2 {
		t.Fatal(ps, truncated, err)
	}
}

func TestClauseFiltersStayWithTheirSubject(t *testing.T) {
	p := parseQuestion("赤槐属于山谷吗，灰栎的再部署时间是多少？", []string{"赤槐", "灰栎"})
	if !p.boundClauses || len(p.Constraints) > 0 || len(p.Bindings) != 2 {
		t.Fatal(p)
	}
	for _, b := range p.Bindings {
		if b.Subject == "赤槐" && !slices.Equal(b.Filters, []string{"山谷"}) {
			t.Fatal(b)
		}
		if b.Subject == "灰栎" && len(b.Filters) > 0 {
			t.Fatal(b)
		}
	}
}

func TestFrontedAttributesAndUnanchoredResidueDoNotCreateBindings(t *testing.T) {
	p := parseQuestion("公开招募这项获取渠道，赤槐有没有？", []string{"赤槐"})
	if p.boundClauses || len(p.Constraints) > 0 || len(p.Bindings) != 1 {
		t.Fatal(p)
	}
	p = parseQuestion("赤槐从什么途径获取、出场要几费？", []string{"赤槐"})
	if p.boundClauses {
		t.Fatal("unanchored residue became a new subject", p)
	}
}

func TestClauseCanonicalAttributesAreLocal(t *testing.T) {
	p := parseQuestion("赤槐是否优先攻击空中单位，以及灰栎能否对空？", []string{"赤槐", "灰栎"})
	if !p.boundClauses || len(p.Bindings) != 2 || p.Bindings[0].Attribute != "优先攻击空中单位" || p.Bindings[1].Attribute != "对空" || len(p.Requirements) != 2 {
		t.Fatal(p)
	}
}
