package search

import (
	"slices"
	"testing"
)

func TestRequestComplementsPreserveSubjectAndConditions(t *testing.T) {
	for _, name := range []string{"赤槐", "无忧", "12F"} {
		for _, request := range []string{"说清楚一点", "讲明白一些", "报清楚点", "再告诉我们", "然后告诉我"} {
			p := parseQuestion(name+"的血量上限，"+request+"。", []string{name})
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 || p.Attribute != "生命上限" {
				t.Fatal(request, p)
			}
		}
	}
	for _, tc := range []struct{ question, attribute string }{
		{"赤槐的再次部署耗时是多少？", "再部署时间"},
		{"赤槐的重新登场用时报给我。", "再部署时间"},
		{"赤槐起手要几费？", "初始部署费用"},
		{"赤槐的起手成本再告诉我。", "初始部署费用"},
		{"赤槐的血量值是多少？", "生命上限"},
	} {
		p := parseQuestion(tc.question, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || p.Attribute != tc.attribute || len(p.Constraints) != 0 {
			t.Fatal(tc.question, p)
		}
	}
	for _, mode := range []string{"逐个", "对照着", "对照"} {
		p := parseQuestion("赤槐和蓝杉的血量上限，"+mode+"报出来。", []string{"赤槐", "蓝杉"})
		if !slices.Equal(p.Subjects, []string{"赤槐", "蓝杉"}) || len(p.Constraints) != 0 {
			t.Fatal(mode, p)
		}
	}
	for _, q := range []string{"赤槐的血量上限再增加一点，告诉我。", "赤槐起手攻击力是多少？", "赤槐的血量上限至少100点，讲清楚一点。", "赤槐模组后的血量上限再告诉我。"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
	for _, name := range []string{"一点", "再生", "逐个"} {
		p := parseQuestion(name+"的血量上限是多少？", []string{name})
		if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
			t.Fatal("real name lost", name, p)
		}
	}
	p := parseQuestion("灰栎的血量上限，讲清楚一点。", nil)
	if !slices.Equal(p.Subjects, []string{"灰栎"}) || len(p.Constraints) != 0 {
		t.Fatal("unknown subject lost", p)
	}
}
