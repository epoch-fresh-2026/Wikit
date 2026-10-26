package search

import (
	"slices"
	"strings"
	"testing"
)

func TestRequestObjectNormalization(t *testing.T) {
	for _, q := range []string{
		"请给出青榆和赤杉的阻挡数，再把两名角色各自的获得方式列出。",
		"把青榆的获得方式核实清楚后，请继续查询她的阻挡数。",
		"请核实青榆的所属势力，再将其攻击间隔一并列出。",
		"请先完成青榆获得方式的核验，再查询他的初始部署费用。",
		"请逐条核对青榆的阻挡数，并顺便查询该干员的再部署时间。",
		"请核验青榆的所属势力，接下来继续给出其攻击间隔。",
		"请列出青榆和赤杉的初始部署费用，然后补充这二人的再部署时间。",
	} {
		for _, known := range [][]string{nil, {"青榆", "赤杉"}} {
			p := parseQuestion(q, known)
			want := 2
			if strings.Contains(q, "赤杉") {
				want = 4
			}
			if len(p.Bindings) != want || len(p.Constraints) > 0 {
				t.Fatal(q, known, p)
			}
		}
	}
}

func TestRelocatedRequestOrganization(t *testing.T) {
	for _, q := range []string{
		"把青榆的阻挡数给我们一并报出来。",
		"把青榆的阻挡数给咱说明白。",
		"请把青榆与赤杉的生命上限并排列出来。",
		"把青榆与赤杉的初始部署费用并排报给我们。",
	} {
		for _, known := range [][]string{nil, {"青榆", "赤杉"}} {
			p := parseQuestion(q, known)
			want := 1
			if strings.Contains(q, "赤杉") {
				want = 2
			}
			if len(p.Bindings) != want || len(p.Constraints) > 0 {
				t.Fatal(q, known, p)
			}
		}
	}
}

func TestCompactReferencesAndRequestBoundaries(t *testing.T) {
	for _, q := range []string{
		"请查青榆的攻击间隔，再给出其所属势力。",
		"请查青榆的攻击间隔，再给出其的所属势力与其阻挡数。",
		"请查青榆的生命上限，并补充其精英二的攻击。",
		"请完成青榆与赤杉的获得方式的查询，再列出这两人的阻挡数。",
		"请给出青榆与赤杉的阻挡数，再将两名对象各自的获得方式列出？",
	} {
		p := parseQuestion(q, []string{"青榆", "赤杉"})
		if len(p.Constraints) > 0 || len(p.Subjects) > 2 {
			t.Fatal(q, p)
		}
	}
	for _, q := range []string{"其攻击间隔是多少？", "青榆和赤杉的阻挡数与其攻击间隔是多少？", "青榆的获得方式及这二人的阻挡数是什么？"} {
		p := parseQuestion(q, nil)
		if !slices.ContainsFunc(p.Subjects, func(s string) bool { return unresolvedReference(p, s) }) {
			t.Fatal(q, p)
		}
	}
	for _, q := range []string{"请完成青榆攻击提升20%后的数值的核验，再查其生命上限。", "青榆的攻击受到核实清楚后效果影响是多少？", "请查青榆的其他效果对攻击的影响。"} {
		p := parseQuestion(q, []string{"青榆"})
		if len(p.Constraints) == 0 {
			t.Fatal("lost condition", q, p)
		}
	}
	for _, name := range []string{"其", "这二人", "完成", "顺便", "接下来", "逐条核对"} {
		p := parseQuestion(name+"的阻挡数是多少？", []string{name})
		if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) > 0 {
			t.Fatal(name, p)
		}
	}
}
