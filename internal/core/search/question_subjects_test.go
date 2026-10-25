package search

import (
	"fmt"
	"slices"
	"testing"
)

func TestUnknownPossessiveSubjectsSurviveRequestFrames(t *testing.T) {
	for _, name := range []string{"灰栎", "未央", "无忧", "12F", "灰1", "UnknownZX12"} {
		for _, frame := range []string{"给我列出来%s的HP。", "说明一下%s的开局费用。", "%s的HP，给我报一下。", "把%s的HP报给我。"} {
			q := fmt.Sprintf(frame, name)
			p := parseQuestion(q, nil)
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 || p.Scope != "named" {
				t.Fatalf("%s: %+v", q, p)
			}
		}
	}
	p := parseQuestion("给我列出来灰栎和蓝杉的HP。", nil)
	if !slices.Equal(p.Subjects, []string{"灰栎", "蓝杉"}) || len(p.Constraints) != 0 {
		t.Fatal("parallel unknown subject disappeared", p)
	}
	for _, q := range []string{"给我说明一下生命值。", "生命值报给我。", "给我列出模组后的HP。"} {
		p := parseQuestion(q, nil)
		if len(p.Subjects) != 0 || len(p.Constraints) == 0 {
			t.Fatal("attribute or modifier became a subject", q, p)
		}
	}
	for _, prefix := range []string{"所有", "全部", "哪些", "这些"} {
		p := parseQuestion("把"+prefix+"样本的出场费用列出来。", nil)
		if len(p.Subjects) != 0 || p.Scope != "collection" || len(p.Constraints) != 0 {
			t.Fatal("collection phrase became a subject", p)
		}
	}
	p = parseQuestion("赤槐与蓝杉的HP，麻烦各报一下。", []string{"赤槐", "蓝杉"})
	if !slices.Equal(p.Subjects, []string{"赤槐", "蓝杉"}) || len(p.Constraints) != 0 {
		t.Fatal("distributive residue became a subject", p)
	}
	p = parseQuestion("给我列出灰栎的HP至少100点。", nil)
	if !slices.Equal(p.Subjects, []string{"灰栎"}) || !slices.Contains(p.Constraints, "numeric:100") {
		t.Fatal("actual numeric condition disappeared", p)
	}
}

func TestCompoundRequestAndAttributeFamilies(t *testing.T) {
	p := parseQuestion("赤槐模组后的HP说明一下。", []string{"赤槐", "说明"})
	if !p.natural || !slices.Equal(p.Subjects, []string{"赤槐"}) || !slices.Contains(p.Constraints, "unparsed_modifier:模组后") {
		t.Fatal("compound command stem was mistaken for a readable title", p)
	}
	for _, alias := range []string{"HP上限", "hp上限", "hP总量", "开局费用", "开局成本"} {
		for _, operator := range []string{"说明一下", "说清楚", "解释一下", "瞅瞅", "报给我", "列给我们"} {
			p := parseQuestion("赤槐的"+alias+operator+"。", []string{"赤槐"})
			if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
				t.Fatal(alias, operator, p)
			}
		}
	}
	for _, q := range []string{"PHP上限是什么？", "UnknownHP上限是多少？"} {
		if parseQuestion(q, nil).natural {
			t.Fatal("HP matched inside an identifier", q)
		}
	}
}
