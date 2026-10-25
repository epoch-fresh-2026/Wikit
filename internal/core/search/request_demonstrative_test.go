package search

import (
	"slices"
	"testing"
)

func TestRequestFramesKeepSubjectArguments(t *testing.T) {
	for _, q := range []string{
		"请核对青榆的攻击间隔资料。", "请分别核实青榆的攻击间隔。", "我们希望了解青榆的攻击间隔情况。",
		"我想查询青榆的攻击间隔数据。", "请帮我查证青榆的攻击间隔信息。", "青榆的攻击间隔，请确认一下。",
	} {
		for _, known := range [][]string{nil, {"青榆"}} {
			p := parseQuestion(q, known)
			if !slices.Equal(p.Subjects, []string{"青榆"}) || len(p.Constraints) > 0 || len(p.Bindings) != 1 || p.Bindings[0].Attribute != "攻击间隔" {
				t.Fatal(q, p)
			}
		}
	}
}

func TestDemonstrativeSubjectBindings(t *testing.T) {
	for _, reference := range []string{"这位干员", "那名角色", "该干员", "此对象", "这位"} {
		for _, known := range [][]string{nil, {"青榆"}} {
			p := parseQuestion("青榆的获得方式和"+reference+"的攻击间隔分别是什么？", known)
			if !slices.Equal(p.Subjects, []string{"青榆"}) || len(p.Bindings) != 2 || len(p.Constraints) > 0 {
				t.Fatal(reference, p)
			}
		}
	}
	for _, reference := range []string{"这两位", "那两名干员", "此二个对象", "这2位干员", "这些干员"} {
		p := parseQuestion("青榆和赤杉的获得方式以及"+reference+"的阻挡数是什么？", nil)
		if len(p.Subjects) != 2 || len(p.Bindings) != 4 || len(p.Constraints) > 0 {
			t.Fatal(reference, p)
		}
	}
}

func TestDemonstrativeAmbiguityAndCardinality(t *testing.T) {
	for _, q := range []string{"这位干员的攻击间隔是多少？", "青榆的获得方式和这两位的攻击间隔是什么？", "青榆和赤杉的获得方式以及该干员的攻击间隔是什么？", "青榆和赤杉的获得方式以及这三位的攻击间隔是什么？", "该干员的攻击间隔和青榆的获得方式是什么？"} {
		p := parseQuestion(q, nil)
		c := assessCoverage(p, nil, nil, nil, false)
		found := false
		for _, r := range c.Requirements {
			if unresolvedReference(p, r.Subject) {
				found = true
				if r.Status != "unverified" || slices.Contains(c.MissingSubjects, r.Subject) {
					t.Fatal(q, c)
				}
			}
		}
		if !found {
			t.Fatal(q, p)
		}
	}
}

func TestRequestAndDemonstrativeNameProtection(t *testing.T) {
	for _, name := range []string{"核对", "核对站", "这位干员", "青榆核实"} {
		p := parseQuestion(name+"的攻击间隔是多少？", []string{name})
		if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) > 0 {
			t.Fatal(name, p)
		}
	}
	for _, q := range []string{"核对青榆受到确认效果后的攻击是多少？", "请核对青榆的攻击数据提升20%后是多少？", "青榆的获得方式和该干员受到攻击提升30%后的攻击是多少？"} {
		p := parseQuestion(q, []string{"青榆"})
		if len(p.Constraints) == 0 {
			t.Fatal("condition dropped", q, p)
		}
	}
	p := parseQuestion("青榆精英一的生命上限和这位干员精英二的攻击是多少？", []string{"青榆"})
	if len(p.Bindings) != 2 || len(p.Constraints) > 0 || p.Bindings[0].Stage == nil || p.Bindings[1].Stage == nil || *p.Bindings[0].Stage != "精英1" || *p.Bindings[1].Stage != "精英2" {
		t.Fatal(p)
	}
}
