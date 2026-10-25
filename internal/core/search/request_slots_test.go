package search

import (
	"fmt"
	"slices"
	"testing"
)

func TestComposedRequestPositionsAndPrefixes(t *testing.T) {
	names := []string{"报春", "否极", "这条河"}
	for _, name := range names {
		for _, operator := range requestOperators() {
			for _, frame := range []string{"请帮我%s%s的部署价格吧。", "%s%s的部署价格。", "%s的部署价格呀，麻烦%s。", "%s的部署价格%s吧，谢谢。"} {
				var q string
				if frame == "%s的部署价格呀，麻烦%s。" || frame == "%s的部署价格%s吧，谢谢。" {
					q = fmt.Sprintf(frame, name, operator)
				} else {
					q = fmt.Sprintf(frame, operator, name)
				}
				p := parseQuestion(q, names)
				if !p.natural || p.Attribute != "初始部署费用" || !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
					t.Fatalf("%s: %+v", q, p)
				}
			}
		}
	}
}

func TestAttributeDemonstrativesAndPolarityRemainScoped(t *testing.T) {
	names := []string{"报春", "否极"}
	for _, deictic := range []string{"这", "那", "该", "此"} {
		for _, classifier := range []string{"条", "种", "项", "个"} {
			p := parseQuestion("报春有没有公开招募"+deictic+classifier+"获取渠道？", names)
			if !slices.Equal(p.Subjects, names[:1]) || len(p.Constraints) != 0 {
				t.Fatal(p)
			}
		}
	}
	for _, form := range []string{"能否", "可否", "是否", "可以否"} {
		p := parseQuestion("否极"+form+"对空", names)
		if !p.natural || !slices.Equal(p.Subjects, names[1:]) || len(p.Constraints) != 0 {
			t.Fatal(p)
		}
		p = parseQuestion("否极"+form+"不对空？", names)
		if !slices.Contains(p.Constraints, "negation") {
			t.Fatal("outer negation lost", p)
		}
	}
	for _, condition := range []string{"受这条效果影响", "这条未知修正", "模组后", "明年", "至少100", "不能"} {
		p := parseQuestion("请帮我报一下报春"+condition+"的部署价格吧。", names)
		if !slices.Equal(p.Subjects, names[:1]) || len(p.Constraints) == 0 {
			t.Fatal("real condition lost", condition, p)
		}
	}
	p := parseQuestion("报春和灰栎的部署价格，麻烦报一下。", names)
	if !slices.Equal(p.Subjects, []string{"报春", "灰栎"}) {
		t.Fatal("missing subject lost", p)
	}
}
