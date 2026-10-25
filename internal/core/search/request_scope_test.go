package search

import (
	"slices"
	"testing"
)

func TestCoordinatedInformationNouns(t *testing.T) {
	for _, q := range []string{"请核实青榆的阻挡数与攻击间隔资料。", "青榆的生命上限和攻击数据分别是多少？", "请核查青榆的获得方式、赤杉的攻击间隔信息。"} {
		for _, known := range [][]string{nil, {"青榆", "赤杉"}} {
			p := parseQuestion(q, known)
			if len(p.Constraints) > 0 || len(p.Bindings) != 2 || slices.Contains(p.Subjects, "资料") || slices.Contains(p.Subjects, "信息") || slices.Contains(p.Subjects, "数据") {
				t.Fatal(q, p)
			}
		}
	}
}

func TestAnaphoricAndCountedReferences(t *testing.T) {
	for _, reference := range []string{"上述干员", "前述对象", "以上角色"} {
		p := parseQuestion("青榆的获得方式和"+reference+"的攻击间隔是什么？", nil)
		if !slices.Equal(p.Subjects, []string{"青榆"}) || len(p.Bindings) != 2 || len(p.Constraints) > 0 {
			t.Fatal(reference, p)
		}
	}
	for _, reference := range []string{"二者", "两者", "2者", "上述两位干员"} {
		p := parseQuestion("青榆和赤杉的获得方式，以及"+reference+"的再部署时间是什么？", nil)
		if len(p.Subjects) != 2 || len(p.Bindings) != 4 || len(p.Constraints) > 0 {
			t.Fatal(reference, p)
		}
	}
	for _, q := range []string{"上述干员的攻击间隔是多少？", "青榆的获得方式及二者的攻击间隔是什么？", "青榆和赤杉的获得方式及上述干员的攻击间隔是什么？", "二者的获得方式和青榆的攻击间隔是什么？"} {
		p := parseQuestion(q, nil)
		if !slices.ContainsFunc(p.Subjects, func(s string) bool { return unresolvedReference(p, s) }) {
			t.Fatal(q, p)
		}
	}
}

func TestCompoundRequestOperations(t *testing.T) {
	for _, q := range []string{
		"请把青榆的获得方式核实后，再给出该干员的攻击间隔。",
		"麻烦将青榆的阻挡数核验之后，随后提供上述干员的生命上限。",
		"先确认青榆的阻挡数，再核对那位的攻击间隔资料。",
		"请把青榆精英一的生命上限核查后，接着返回前述干员精英二的攻击。",
	} {
		for _, known := range [][]string{nil, {"青榆"}} {
			p := parseQuestion(q, known)
			if !slices.Equal(p.Subjects, []string{"青榆"}) || len(p.Bindings) != 2 || len(p.Constraints) > 0 {
				t.Fatal(q, p)
			}
		}
	}
}

func TestCompoundRequestsPreserveConditionsAndNames(t *testing.T) {
	for _, q := range []string{"请把青榆的攻击提升20%后的数值核查后，再提供该干员的生命上限。", "青榆的攻击受到核验效果影响后是多少？", "请查青榆的攻击与生命数据提升20%后的数值。"} {
		p := parseQuestion(q, []string{"青榆"})
		if len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
	for _, name := range []string{"资料", "上述干员", "二者", "先确认", "核查"} {
		p := parseQuestion(name+"的生命上限是多少？", []string{name})
		if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) > 0 {
			t.Fatal(name, p)
		}
	}
}
