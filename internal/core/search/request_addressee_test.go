package search

import (
	"slices"
	"testing"
)

func TestAddresseeAndDistributionRoles(t *testing.T) {
	for _, phrase := range []string{"一个人一条", "每人一行", "按每个页面一项", "一个页面一行", "每个主体一项"} {
		p := parseQuestion("赤槐和灰栎的HP，"+phrase+"报出来。", []string{"赤槐", phrase})
		if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) || len(p.Constraints) != 0 {
			t.Fatal(phrase, p)
		}
	}
	for _, pronoun := range requestAddressees() {
		for _, prefix := range []string{"给我", "替我们", ""} {
			p := parseQuestion("赤槐的HP，"+pronoun+prefix+"列一下。", []string{"赤槐", pronoun})
			if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
				t.Fatal(pronoun, prefix, p)
			}
		}
	}
	for _, name := range []string{"你", "你给我", "每人一行", "一个人一条"} {
		for _, known := range [][]string{nil, {name}} {
			p := parseQuestion(name+"的HP报个数。", known)
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatal("real subject lost", name, p)
			}
		}
	}
	for _, q := range []string{"赤槐受每人一行效果影响的HP报个数。", "赤槐的HP至少100点，每人一行报出来。", "赤槐模组后的HP，你给我列一下。", "赤槐的HP，一个人一条装备影响后报出来。"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
}

func TestSampleCountDoesNotConsumeOutputQuantities(t *testing.T) {
	for _, frame := range []struct {
		q     string
		count int
	}{
		{"全部样本的HP，一个页面一行列出来。", 0},
		{"5个样本的HP，一个页面一行列出来。", 5},
		{"五个样本干员的HP，每人一行列出来。", 5},
	} {
		p := parseQuestion(frame.q, nil)
		if p.expectedCount != frame.count || len(p.Subjects) != 0 || len(p.Constraints) != 0 || p.Scope != "collection" {
			t.Fatal(frame, p)
		}
	}
}

func TestAttributeAndRequestFamiliesWithParticles(t *testing.T) {
	for _, q := range []string{"赤槐的满血有多少点？", "赤槐的血量总值报个数。", "赤槐再上场的耗时多久？", "赤槐起手的成本告诉我们就行。", "帮我把赤槐的阻挡数报个数。"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 || !p.natural {
			t.Fatal(q, p)
		}
	}
	p := parseQuestion("赤槐的生命值至少2000点，你给我报个数就行。", []string{"赤槐"})
	if !slices.Contains(p.Constraints, "numeric:2000") {
		t.Fatal("numeric qualifier lost", p)
	}
	p = parseQuestion("赤槐满血时的攻击力是多少？", []string{"赤槐"})
	if p.Attribute != "攻击" || len(p.Constraints) == 0 {
		t.Fatal("full-health condition became a life amount", p)
	}
}
