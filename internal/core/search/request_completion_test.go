package search

import (
	"slices"
	"testing"
)

func TestCompletedAndConnectedRequests(t *testing.T) {
	for _, q := range []string{
		"请逐项核验青榆的阻挡数和再部署时间信息。",
		"核实完青榆的所属势力后，请继续查询该干员的攻击间隔。",
		"核查好青榆的阻挡数之后，并且给出她的攻击间隔。",
		"请核查青榆的获得方式，并把她的阻挡数一并列出。",
		"请把青榆的所属势力确认好，再给出该干员的攻击间隔。",
		"先查询青榆的阻挡数，并且将她的再部署时间逐项列出。",
	} {
		for _, known := range [][]string{nil, {"青榆"}} {
			p := parseQuestion(q, known)
			if !slices.Equal(p.Subjects, []string{"青榆"}) || len(p.Bindings) != 2 || len(p.Constraints) > 0 {
				t.Fatal(q, p)
			}
		}
	}
}

func TestClassifiedReferences(t *testing.T) {
	for _, ref := range []string{"两名干员各自", "二位角色", "2个对象各自"} {
		p := parseQuestion("请核对青榆和赤杉的再部署时间，并提供"+ref+"的攻击间隔资料。", nil)
		if len(p.Subjects) != 2 || len(p.Bindings) != 4 || len(p.Constraints) > 0 {
			t.Fatal(ref, p)
		}
	}
	for _, q := range []string{"请给出两名干员各自的攻击间隔。", "青榆的获得方式和两名干员各自的阻挡数是什么？", "青榆和赤杉的获得方式与一名干员的攻击间隔是什么？"} {
		p := parseQuestion(q, nil)
		if !slices.ContainsFunc(p.Subjects, func(s string) bool { return unresolvedReference(p, s) }) {
			t.Fatal(q, p)
		}
	}
}

func TestRequestCompletionPreservesConditionsAndNames(t *testing.T) {
	for _, q := range []string{
		"核实完青榆的攻击提升20%后的数值后，请继续查询她的生命上限。",
		"请核查青榆受到确认好效果影响后的攻击。",
		"请查青榆的生命上限，并提供她的攻击降低20%后的数值。",
		"青榆的攻击完成强化后是多少？",
	} {
		p := parseQuestion(q, []string{"青榆"})
		if len(p.Constraints) == 0 {
			t.Fatal("lost condition", q, p)
		}
	}
	for _, name := range []string{"两名干员各自", "核实完", "确认好", "继续查询", "逐项核验", "并把她"} {
		p := parseQuestion(name+"的阻挡数是多少？", []string{name})
		if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) > 0 {
			t.Fatal(name, p)
		}
	}
}
