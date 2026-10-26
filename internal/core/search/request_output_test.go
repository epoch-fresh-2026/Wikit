package search

import (
	"slices"
	"testing"
)

func TestOutputMannerAndReportingFamilies(t *testing.T) {
	for _, q := range []string{"赤槐的HP，麻烦你简略报一下。", "赤槐起手的成本，简单给个数就行。", "赤槐和灰栎的HP，每人一条分开报出来。", "赤槐与灰栎的阻挡数，按一人一项排列给我。", "赤槐的生命最大值有多少？", "赤槐再次落地时间要多久？"} {
		p := parseQuestion(q, []string{"赤槐", "简略", "排列"})
		want := []string{"赤槐"}
		if p.Comparison {
			want = append(want, "灰栎")
		}
		if !slices.Equal(p.Subjects, want) || len(p.Constraints) != 0 {
			t.Fatal(q, p)
		}
	}
	for _, alias := range []string{"血量最大值", "HP最大值", "血条最大值", "重新落地的用时", "再落地耗时"} {
		p := parseQuestion("蓝杉的"+alias+"给个结果。", nil)
		if !slices.Equal(p.Subjects, []string{"蓝杉"}) || len(p.Constraints) != 0 {
			t.Fatal(alias, p)
		}
	}
	for _, name := range []string{"简略", "每人一条分开", "分开"} {
		for _, known := range [][]string{nil, {name}} {
			p := parseQuestion(name+"的HP给个数。", known)
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatal("subject lost", name, p)
			}
		}
	}
	p := parseQuestion("排列的HP，简略给个数。", []string{"排列"})
	if !slices.Equal(p.Subjects, []string{"排列"}) || len(p.Constraints) != 0 {
		t.Fatal("compound title lost", p)
	}
	for _, q := range []string{"赤槐受简略效果影响的HP给个数。", "赤槐的HP分开治疗后给个数。", "赤槐的HP至少100点，简略给个数。", "赤槐模组后的HP，每人一条分开报出来。"} {
		p := parseQuestion(q, []string{"赤槐", "简略"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
	p = parseQuestion("赤槐再次落地后的攻击力，给个数。", []string{"赤槐"})
	if p.Attribute != "攻击" || len(p.Constraints) == 0 {
		t.Fatal("deployment condition became time", p)
	}
}
