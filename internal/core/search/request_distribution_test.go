package search

import (
	"slices"
	"testing"
)

func TestDistributionRequestsDoNotInventSubjects(t *testing.T) {
	for _, unit := range []string{"人", "位", "个主体", "名干员", "个干员", "个页面", "个对象"} {
		for _, output := range []string{"条", "行", "项"} {
			phrase := "一" + unit + "一" + output
			for _, request := range []string{"报出来", "讲得明白一点", "再告诉我们"} {
				p := parseQuestion("赤槐和灰栎的HP，"+phrase+request+"。", []string{"赤槐"})
				if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) || len(p.Constraints) != 0 {
					t.Fatal(phrase, request, p)
				}
				p = parseQuestion("赤槐和灰栎的HP，"+phrase+request+"。", []string{"赤槐", phrase})
				if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) || len(p.Constraints) != 0 {
					t.Fatal("organization matched an unrelated title", phrase, p)
				}
			}
		}
	}
	for _, name := range []string{"一人一条", "一位一行", "并排", "开场"} {
		for _, known := range [][]string{nil, {name}} {
			p := parseQuestion(name+"的HP给我报出来。", known)
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatal("subject removed", name, p)
			}
		}
	}
	for _, q := range []string{"赤槐受一人一条效果影响的HP报出来。", "赤槐的HP，一人一条装备影响后报出来。", "赤槐的HP至少100点，一人一条报出来。", "赤槐模组后的HP一人一条报出来。"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
}

func TestOptionalRequestAndAttributeParticles(t *testing.T) {
	for _, verb := range []string{"说", "讲", "报", "答"} {
		for _, complement := range []string{"清楚", "明白"} {
			p := parseQuestion("赤槐的HP，"+verb+"得"+complement+"一点。", []string{"赤槐"})
			if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
				t.Fatal(p)
			}
		}
	}
	for _, head := range []string{"部署", "出场", "起手", "开场"} {
		for _, tail := range []string{"费用", "成本", "开销", "价"} {
			p := parseQuestion("赤槐"+head+"的"+tail+"报给我们。", []string{"赤槐"})
			if !slices.Equal(p.Subjects, []string{"赤槐"}) || p.Attribute != "初始部署费用" || len(p.Constraints) != 0 {
				t.Fatal(head, tail, p)
			}
		}
	}
	for _, q := range []string{"赤槐开场得花几费？", "赤槐的HP并排列出来。"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
			t.Fatal(q, p)
		}
	}
	p := parseQuestion("赤槐开场攻击力是多少？", []string{"赤槐"})
	if len(p.Constraints) == 0 {
		t.Fatal("attribute qualifier removed", p)
	}
}
