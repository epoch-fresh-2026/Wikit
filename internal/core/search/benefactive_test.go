package search

import (
	"fmt"
	"slices"
	"testing"
)

func TestBenefactivePrefixesDoNotIntroduceSubjects(t *testing.T) {
	names := []string{"赤槐", "给我", "给我们", "列出来"}
	for _, prefix := range phraseAliases([]string{"给", "替", "为", "帮"}, []string{"我", "我们", "你", "您"}) {
		for _, operator := range []string{"列出来", "报出来", "讲出去", "报下"} {
			for _, frame := range []string{"赤槐的生命总量，%s%s。", "%s%s赤槐的生命总量。"} {
				q := fmt.Sprintf(frame, prefix, operator)
				p := parseQuestion(q, names)
				if !p.natural || !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
					t.Fatalf("%s: %+v", q, p)
				}
			}
		}
	}
	for _, name := range names[1:] {
		p := parseQuestion("把"+name+"的HP给我们报出来吧。", names)
		if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
			t.Fatal("benefactive/title collision", name, p)
		}
	}
	p := parseQuestion("赤槐与灰栎的HP，给我重新整理。", names)
	if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) || len(p.Constraints) == 0 {
		t.Fatal("unknown request invented a subject or lost a real one", p)
	}
	p = parseQuestion("赤槐受给我效果影响的HP，给我们报出来。", names)
	if !slices.Equal(p.Subjects, []string{"赤槐"}) || !slices.Contains(p.Constraints, "unparsed_modifier:受给我效果影响") {
		t.Fatal("named effect became a comparison subject or disappeared", p)
	}
}

func TestQuantityIntentAndRequestOrganizationPreserveConditions(t *testing.T) {
	for _, action := range []string{"挡", "拦"} {
		for _, quantity := range []string{"几个", "多少个", "多少"} {
			for _, target := range []string{"怪", "怪物", "敌人"} {
				p := parseQuestion("赤槐能"+action+quantity+target+"？", []string{"赤槐"})
				if p.Attribute != "阻挡数" || !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
					t.Fatal(action, quantity, target, p)
				}
			}
		}
	}
	for _, mode := range []string{"按人", "按主体", "按对象", "一并", "一起"} {
		p := parseQuestion("赤槐和蓝杉的生命总量，给我"+mode+"列出来。", []string{"赤槐", "蓝杉"})
		if !slices.Equal(p.Subjects, []string{"赤槐", "蓝杉"}) || len(p.Constraints) != 0 {
			t.Fatal(mode, p)
		}
	}
	for _, condition := range []string{"模组后", "明年", "至少100", "受给我效果影响", "按人头加成后", "不能降低"} {
		p := parseQuestion("赤槐"+condition+"的生命总量，给我列出来。", []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("true condition lost", condition, p)
		}
	}
	p := parseQuestion("赤槐最多能挡几个怪？", []string{"赤槐"})
	if len(p.Constraints) == 0 {
		t.Fatal("superlative constraint lost", p)
	}
}
