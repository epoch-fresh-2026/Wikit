package search

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestDisposalRequestsPreserveNamesAndUnknownConditions(t *testing.T) {
	names := []string{"把酒", "将离", "列出"}
	for _, name := range names {
		for _, marker := range []string{"把", "将"} {
			for _, attribute := range []string{"出场价", "落地价", "上场价", "部署价格"} {
				for _, operator := range []string{"报下", "报出", "列下", "列出"} {
					for _, separator := range []string{"", "，帮我"} {
						q := fmt.Sprintf("请%s%s的%s%s%s吧。", marker, name, attribute, separator, operator)
						p := parseQuestion(q, names)
						if !p.natural || !slices.Equal(p.Subjects, []string{name}) || p.Attribute != "初始部署费用" || len(p.Constraints) != 0 {
							t.Fatalf("%s: %+v", q, p)
						}
					}
				}
			}
		}
	}
	for _, condition := range []string{"模组后", "明年", "至少100", "不能降低", "受报下效果影响"} {
		p := parseQuestion("把将离"+condition+"的HP报下吧。", names)
		if !slices.Equal(p.Subjects, []string{"将离"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", condition, p)
		}
	}
}

func TestRequestResiduesNeverBecomeParallelSubjects(t *testing.T) {
	p := parseQuestion("报春的部署价格列出吧。", []string{"报春", "列出"})
	if !p.natural || !slices.Equal(p.Subjects, []string{"报春"}) || len(p.Constraints) != 0 {
		t.Fatal("title alias shadowed a tail imperative", p)
	}
	for _, verb := range []string{"列明", "汇总", "分析"} {
		p := parseQuestion("赤槐和蓝杉的生命点数，帮我分别"+verb+"。", []string{"赤槐", "蓝杉"})
		if !slices.Equal(p.Subjects, []string{"赤槐", "蓝杉"}) || !slices.Contains(p.Constraints, "unparsed_modifier:"+verb) {
			t.Fatal("request residue invented a subject or disappeared", p)
		}
	}
	p = parseQuestion("把赤槐和灰栎的出场价，帮我分别列出吧。", []string{"赤槐"})
	if !slices.Equal(p.Subjects, []string{"赤槐", "灰栎"}) || len(p.Constraints) != 0 {
		t.Fatal("true missing subject lost", p)
	}
	for _, q := range []string{"赤槐的出场价值是多少？", "赤槐把HP降低多少？"} {
		p := parseQuestion(q, []string{"赤槐"})
		if len(p.Constraints) == 0 {
			t.Fatal("abbreviation or disposal marker dropped a condition", p)
		}
		if strings.Contains(q, "降低") && !slices.Contains(p.Constraints, "unparsed_modifier:把") {
			t.Fatal("disposal marker removed outside request context", p)
		}
	}
}
