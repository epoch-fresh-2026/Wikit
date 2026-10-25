package search

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestCompositionalQuestionsProtectNamesAndRemoveGrammar(t *testing.T) {
	names := []string{"赤槐", "蓝杉", "和风"}
	for _, name := range names {
		for _, field := range []struct{ alias, attribute string }{{"血量", "生命上限"}, {"部署成本", "初始部署费用"}, {"攻速", "攻击间隔"}, {"阵营", "所属势力"}} {
			q := fmt.Sprintf("请问%s的%s大概是多少呀？", name, field.alias)
			p := parseQuestion(q, names)
			if !slices.Equal(p.Subjects, []string{name}) || p.Attribute != field.attribute {
				t.Fatalf("%s: %+v", q, p)
			}
		}
	}
	for _, separator := range []string{"和", "跟", "与", "、", "以及"} {
		p := parseQuestion("赤槐"+separator+"蓝杉分别谁在精英四满级时攻击更高？", names)
		if !slices.Equal(p.Subjects, []string{"赤槐", "蓝杉"}) || p.Stage == nil || *p.Stage != "精英4" || !p.Comparison {
			t.Fatal(p)
		}
	}
	p := parseQuestion("赤槐和灰栎的攻击间隔是多少？", names)
	if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) {
		t.Fatal("unknown subject removed", p)
	}
	p = parseQuestion("赤槐属于研究所A7吗？", names)
	if !slices.Equal(p.Subjects, []string{"赤槐"}) || !slices.Equal(p.Filters, []string{"研究所A7"}) || len(p.Constraints) > 0 {
		t.Fatal("predicate value misread as a subject or number", p)
	}
	p = parseQuestion("UnknownZX12的血量是多少？", names)
	if !slices.Equal(p.Subjects, []string{"UnknownZX12"}) {
		t.Fatal(p)
	}
}

func TestQuestionConstraintsAndLiteralFallback(t *testing.T) {
	for _, q := range []string{"\"攻击间隔 1.0s\"", "赤槐的未知技能持续多久？", "alpha beta unknown", "法术伤害"} {
		p := parseQuestion(q, nil)
		if p.natural || p.focus != q {
			t.Fatal(p)
		}
	}
	p := parseQuestion("赤槐的攻击间隔是1.0s吗？", []string{"赤槐"})
	if !slices.Contains(p.Constraints, "numeric:1.0s") || !slices.Equal(p.Subjects, []string{"赤槐"}) {
		t.Fatal(p)
	}
	p = parseQuestion("赤槐是不是先打空中单位？", []string{"赤槐"})
	if slices.Contains(p.Constraints, "negation") {
		t.Fatal("binary question misread as negation", p)
	}
	p = parseQuestion("赤槐不能公开招募吗？", []string{"赤槐"})
	if !slices.Contains(p.Constraints, "negation") {
		t.Fatal("negation was lost", p)
	}
}

func TestLongestTokensAndAliasBoundaries(t *testing.T) {
	if residue := strings.TrimSpace(stripLanguage("打远处", []string{"打远", "打", "远处"})); residue != "" {
		t.Fatal("overlapping tokens left a fragment", residue)
	}
	if residue := strings.TrimSpace(stripLanguage("时间数量", []string{"时", "时间", "数", "数量"})); residue != "" {
		t.Fatal(residue)
	}
	for _, name := range []string{"时雨", "比尔", "要离"} {
		for _, alias := range []string{"HP", "hp", "部署开销", "阻挡数量", "返场时间"} {
			q := fmt.Sprintf("帮我查一下%s的%s呗？", name, alias)
			p := parseQuestion(q, []string{name})
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatalf("%s: %+v", q, p)
			}
		}
	}
	if parseQuestion("PHP是谁？", nil).natural {
		t.Fatal("HP matched inside PHP")
	}
	if parseQuestion("UnknownHP2是什么？", nil).natural {
		t.Fatal("HP matched inside an identifier")
	}
	p := parseQuestion("请问时雨再上场的等待时间是多少？", []string{"时雨"})
	if len(p.Constraints) != 0 || !slices.Equal(p.Subjects, []string{"时雨"}) {
		t.Fatal(p)
	}
}

func TestUnknownModifiersAreConstraintsNotInventedSubjects(t *testing.T) {
	if p := parseQuestion("HP是多少？", nil); !slices.Contains(p.Constraints, "subject_unspecified") {
		t.Fatal("missing subject became an implicit collection", p)
	}
	p := parseQuestion("赤槐的神秘修正血量是多少？", []string{"赤槐"})
	if !slices.Equal(p.Subjects, []string{"赤槐"}) || !slices.Contains(p.Constraints, "unparsed_modifier:神秘修正") {
		t.Fatal(p)
	}
	p = parseQuestion("这些样本里赤槐和灰栎的HP分别是多少？", []string{"赤槐"})
	if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) {
		t.Fatal("parallel unknown name lost", p)
	}
	p = parseQuestion("赤槐是否优先攻击空中单位？", []string{"赤槐"})
	if p.Attribute != "优先攻击空中单位" {
		t.Fatal(p)
	}
	p = parseQuestion("赤槐能对空吗？", []string{"赤槐"})
	if p.Attribute != "对空" {
		t.Fatal(p)
	}
	p = parseQuestion("赤槐和蓝杉，谁的HP受未知修正？", []string{"赤槐", "蓝杉"})
	if !slices.Equal(p.Subjects, []string{"赤槐", "蓝杉"}) || len(p.Constraints) == 0 {
		t.Fatal("question clause invented a subject or lost modifiers", p)
	}
}

func TestAttributeWordFamiliesWithRequestFrames(t *testing.T) {
	names := []string{"麻烦树", "好雨", "更大"}
	for _, family := range []struct {
		heads, tails []string
		attribute    string
	}{
		{[]string{"出场", "上场", "部署"}, []string{"费用", "成本", "开销", "花销", "消耗"}, "初始部署费用"},
		{[]string{"获得", "获取", "招募"}, []string{"方式", "渠道", "途径"}, "获得方式"},
		{[]string{"生命"}, []string{"点数", "值", "上限"}, "生命上限"},
		{[]string{"飞行", "空中"}, []string{"单位", "目标", "敌人", "怪物", "怪"}, "对空"},
	} {
		for _, alias := range phraseAliases(family.heads, family.tails) {
			for _, name := range names {
				for _, frame := range []string{"麻烦您查查%s的%s好吗？", "劳驾讲讲%s的%s可以吗？", "烦请看看%s的%s呢？"} {
					q := fmt.Sprintf(frame, name, alias)
					p := parseQuestion(q, names)
					if p.Attribute != family.attribute || !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
						t.Fatalf("%s: %+v", q, p)
					}
				}
			}
		}
	}
	for _, q := range []string{"好雨能攻击天上的飞行怪吗？", "好雨重新上场需要等多久？"} {
		p := parseQuestion(q, names)
		if !slices.Equal(p.Subjects, []string{"好雨"}) || len(p.Constraints) != 0 {
			t.Fatalf("%s: %+v", q, p)
		}
	}
}

func TestConversationalGrammarKeepsApplicabilityConstraints(t *testing.T) {
	names := []string{"好雨", "蓝杉"}
	for _, ending := range []string{"谁的生命点数更大", "谁的阻挡数量更多", "各需要多久重新上场", "逐一对比出场成本"} {
		q := "好雨与蓝杉，" + ending + "？"
		p := parseQuestion(q, names)
		if !slices.Equal(p.Subjects, names) || len(p.Constraints) != 0 || !p.Comparison {
			t.Fatalf("%s: %+v", q, p)
		}
	}
	for _, modifier := range []string{"明年", "加模组后", "受好运效果影响", "至少100点", "大于100", "没有", "不低于100"} {
		q := fmt.Sprintf("麻烦查查好雨%s的生命点数好吗？", modifier)
		p := parseQuestion(q, names)
		if !slices.Equal(p.Subjects, []string{"好雨"}) || len(p.Constraints) == 0 {
			t.Fatalf("applicability constraint disappeared: %s: %+v", q, p)
		}
	}
	p := parseQuestion("劳驾讲讲好雨与灰栎的生命点数好吗？", names)
	if !slices.Equal(p.Subjects, []string{"好雨", "灰栎"}) || len(p.Constraints) != 0 {
		t.Fatal("unknown parallel subject disappeared", p)
	}
}
