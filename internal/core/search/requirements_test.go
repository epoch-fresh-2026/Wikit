package search

import (
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"slices"
	"testing"
)

func TestCoordinatedRequirementsUseVocabularyNotQuestionTemplates(t *testing.T) {
	for _, tc := range []struct {
		q     string
		attrs []string
	}{
		{"赤槐的获得方式和攻击间隔分别是什么？", []string{"获得方式", "攻击间隔"}},
		{"灰栎的伤害类型与获得方式，分别说明一下。", []string{"伤害", "获得方式"}},
		{"赤槐与灰栎的生命上限、攻击力和阻挡数分别是多少？", []string{"生命上限", "攻击", "阻挡数"}},
		{"赤槐的攻击间隔和攻速是多少？", []string{"攻击间隔"}},
		{"赤槐的再部署时间和初始部署费用是多少？", []string{"再部署时间", "初始部署费用"}},
	} {
		p := parseQuestion(tc.q, []string{"赤槐", "灰栎"})
		got := []string{}
		for _, r := range p.Requirements {
			got = append(got, r.Attribute)
		}
		slices.Sort(got)
		slices.Sort(tc.attrs)
		if !slices.Equal(got, tc.attrs) || len(p.Constraints) > 0 {
			t.Errorf("%s: %+v", tc.q, p)
		}
	}
	p := parseQuestion("赤槐的攻击和灰栎的生命上限分别是多少？", []string{"赤槐", "灰栎"})
	if len(p.Bindings) != 2 || len(p.Constraints) > 0 {
		t.Fatal(p)
	}
}

func TestCoverageRequiresEachAttributeOfEachSubjectInReturnedRevision(t *testing.T) {
	p := parseQuestion("赤槐和灰栎的获得方式与攻击间隔分别是什么？", []string{"赤槐", "灰栎"})
	items := []Hit{{PageID: "1", RevisionID: "r1", Title: "赤槐"}, {PageID: "2", RevisionID: "r2", Title: "灰栎"}}
	evidence := []Evidence{
		{CitationID: "E1", PageID: "1", RevisionID: "r1", Passage: Passage{Text: "攻击间隔 1.2s"}},
		{CitationID: "E2", PageID: "2", RevisionID: "r2", Passage: Passage{Text: "攻击间隔 2s"}},
		{CitationID: "E3", PageID: "1", RevisionID: "old", Passage: Passage{Text: "获得方式 公开招募"}},
	}
	c := assessCoverage(p, items, evidence, nil, false)
	if c.Status != "partial" || len(c.Requirements) != 4 || c.Subjects[0].Status != "partial" {
		t.Fatal(c)
	}
	for _, r := range c.Requirements {
		if r.Attribute == "获得方式" && (r.Status != "attribute_missing" || len(r.CitationIDs) > 0) {
			t.Fatal(r)
		}
	}
	evidence[2].RevisionID = "r1"
	evidence = append(evidence, Evidence{CitationID: "E4", PageID: "2", RevisionID: "r2", Passage: Passage{Text: "获得方式 标准寻访"}})
	c = assessCoverage(p, items, evidence, nil, false)
	if c.Status != "complete" {
		t.Fatal(c)
	}
	for _, r := range c.Requirements {
		if len(r.CitationIDs) != 1 {
			t.Fatal(r)
		}
	}
	if assessCoverage(p, items, evidence, nil, true).Status == "complete" {
		t.Fatal("budget ignored")
	}
}

func TestAttributePassageBudgetAndLabels(t *testing.T) {
	for _, text := range []string{"", "获得方式", "获得方式："} {
		if attributePresent(Passage{Text: text, SectionPath: []string{"获得方式"}}, "获得方式") {
			t.Fatal("label counted as evidence", text)
		}
	}
	rev := "r"
	src := Source{Page: content.Page{ID: "1", HeadRevisionID: &rev}}
	p := parseQuestion("赤槐的攻击间隔、获得方式、所属势力和生命上限分别是什么？", []string{"赤槐"})
	blocks := []TextBlock{{Text: "攻击间隔 1s", StartLine: 1, EndLine: 1}, {Text: "获得方式 招募", StartLine: 2, EndLine: 2}, {Text: "所属势力 山谷", StartLine: 3, EndLine: 3}, {Text: "生命上限 100", StartLine: 4, EndLine: 4}}
	ps, truncated, err := requirementPassages(src, p, blockText{blocks: blocks})
	if err != nil || len(ps) != 3 || !truncated {
		t.Fatal(ps, truncated, err, p)
	}
	p = parseQuestion("赤槐的攻击间隔与获得方式分别是什么？", []string{"赤槐"})
	ps, truncated, err = requirementPassages(src, p, blockText{blocks: blocks})
	if err != nil || truncated || !slices.ContainsFunc(ps, func(p Passage) bool { return p.Text == "获得方式 招募" }) {
		t.Fatal(ps, truncated, err)
	}
}

func TestRequirementFiltersStagesAndPlanningBudget(t *testing.T) {
	stage := "精英2"
	p := questionSpec{QuestionPlan: QuestionPlan{Subjects: []string{"赤槐"}, Scope: "named", Requirements: []AttributeRequirement{{"攻击", &stage, []string{}}, {"所属势力", nil, []string{"山谷"}}}}, natural: true}
	items := []Hit{{PageID: "1", RevisionID: "r", Title: "赤槐"}}
	evidence := []Evidence{{PageID: "1", RevisionID: "r", CitationID: "E1", Passage: Passage{Text: "攻击 10 20 ——"}}, {PageID: "1", RevisionID: "r", CitationID: "E2", Passage: Passage{Text: "所属势力 海港"}}}
	blocks := map[string][]TextBlock{"1": {{Text: "精英0 精英1 精英2 攻击 10 20 ——"}}}
	c := assessCoverage(p, items, evidence, blocks, false)
	if c.Requirements[0].Status != "stage_unsupported" || c.Requirements[1].Status != "attribute_missing" || c.Status == "complete" {
		t.Fatal(c)
	}
	p = parseQuestion("赤槐的远程攻击、伤害、对空、初始部署费用、再部署时间、攻击间隔、生命上限、阻挡数、所属势力、获得方式和特性分别是什么？", []string{"赤槐"})
	if len(p.Requirements) != 8 || !slices.Contains(p.Constraints, "attribute_budget") {
		t.Fatal(p)
	}
}
