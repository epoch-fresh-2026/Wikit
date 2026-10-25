package search

import "testing"

func TestStageColumnsRequireTheRequestedAttributeValue(t *testing.T) {
	for _, tc := range []struct{ stage, text, want string }{
		{"精英1", "精英0 1级 精英0 满级 精英1 满级 精英4 满级 生命上限 10 20 30 40", "covered"},
		{"精英4", "精英0 1级 精英0 满级 精英1 满级 精英4 满级 生命上限 10 20 30 40", "covered"},
		{"精英4", "精英0 1级 精英0 满级 精英1 满级 精英4 满级 生命上限 10 20 30 ——", "stage_unsupported"},
		{"精英2", "精英0 1级 精英0 满级 精英1 满级 精英4 满级 生命上限 10 20 30 40", "stage_unsupported"},
		{"精英4", "精英0 精英1 精英4 生命上限 unknown", "unverified"},
		{"精英4", "生命上限 40", "unverified"},
	} {
		got := stageSupport([]TextBlock{{Text: tc.text}}, "生命上限", tc.stage)
		if got != tc.want {
			t.Fatalf("%s %s: %s", tc.stage, tc.text, got)
		}
	}
}

func TestCoverageRequiresEveryNamedSubjectAndApplicableStage(t *testing.T) {
	stage := "精英4"
	p := questionSpec{QuestionPlan: QuestionPlan{Subjects: []string{"赤槐", "灰栎"}, Attribute: "生命上限", Scope: "named", Constraints: []string{}}, natural: true}
	h := Hit{PageID: "3", Title: "赤槐"}
	e := Evidence{PageID: "3", Passage: Passage{Text: "生命上限 40"}}
	c := assessCoverage(p, []Hit{h}, []Evidence{e}, nil, false)
	if c.Status != "partial" || len(c.MissingSubjects) != 1 || c.MissingSubjects[0] != "灰栎" {
		t.Fatal(c)
	}
	p.Subjects = []string{"赤槐"}
	p.Stage = &stage
	c = assessCoverage(p, []Hit{h}, []Evidence{e}, map[string][]TextBlock{"3": {{Text: "精英0 精英4 生命上限 10 ——"}}}, false)
	if c.Status != "insufficient" || c.Subjects[0].Status != "stage_unsupported" {
		t.Fatal(c)
	}
	p.Stage = nil
	c = assessCoverage(p, []Hit{h}, []Evidence{e}, nil, true)
	if c.Status == "complete" || !c.Truncated {
		t.Fatal(c)
	}
}

func TestAttributeLabelsAndNeighboringAttributesAreNotValues(t *testing.T) {
	for _, text := range []string{"属性计算器 生命上限 攻击 防御", "攻击间隔 1.2s"} {
		if attributePresent(Passage{Text: text}, "攻击") {
			t.Fatal(text)
		}
	}
}
