package search

import (
	"slices"
	"testing"
)

func TestComposedRequestPrefixesPreserveSubjectSlots(t *testing.T) {
	for _, prefix := range []string{"你直接", "帮我简单", "你们替我们分别", "您请帮我简要"} {
		p := parseQuestion("赤槐和灰栎的HP，"+prefix+"报个数就行。", []string{"赤槐", "简单", "你直接"})
		if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) || len(p.Constraints) != 0 {
			t.Fatal(prefix, p)
		}
	}
	for _, q := range []string{"你帮我把赤槐的首次部署费列一下。", "赤槐和灰栎的血条上限，各人一条报出来。", "赤槐和灰栎的满生命，一个对象对应一行列出。"} {
		p := parseQuestion(q, []string{"赤槐"})
		want := []string{"赤槐"}
		if p.Comparison {
			want = append(want, "灰栎")
		}
		if !slices.Equal(p.Subjects, want) || len(p.Constraints) != 0 {
			t.Fatal(q, p)
		}
	}
	for _, name := range []string{"简单", "你直接", "各人一条", "一个对象对应一行"} {
		for _, known := range [][]string{nil, {name}} {
			p := parseQuestion(name+"的血条上限报给我。", known)
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatal("subject lost", name, p)
			}
		}
	}
	for _, q := range []string{"赤槐受简单效果影响的HP，你直接报一下。", "赤槐的HP至少100点，帮我简单报一下。", "赤槐模组后的HP，各人一条列出来。", "赤槐满生命时的攻击力是多少？", "赤槐首次攻击力是多少？"} {
		p := parseQuestion(q, []string{"赤槐", "简单"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
	for _, alias := range []string{"血条上限", "满生命", "首次部署费", "初次出场成本", "第一次上场的价格"} {
		p := parseQuestion("灰栎的"+alias+"是多少？", nil)
		if !slices.Equal(p.Subjects, []string{"灰栎"}) || len(p.Constraints) != 0 {
			t.Fatal(alias, p)
		}
	}
}
