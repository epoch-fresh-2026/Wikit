package search

import (
	"fmt"
	"slices"
	"testing"
)

func TestPolarQuestionsAndOuterNegation(t *testing.T) {
	names := []string{"无忧", "不语", "未央"}
	for _, name := range names {
		for _, form := range []string{"能不能", "会不会", "是不是", "有没有", "需不需要", "需要不需要", "要不要", "可不可以", "可以不可以"} {
			q := name + form + "对空"
			p := parseQuestion(q, names)
			if !p.natural || !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatalf("polar question or title mistaken for negation: %s: %+v", q, p)
			}
		}
	}
	for _, phrase := range []string{"不能", "不能不能", "能不能不", "会不会没有", "可不可以不", "未", "无"} {
		p := parseQuestion("无忧"+phrase+"对空吗？", names)
		if !slices.Contains(p.Constraints, "negation") {
			t.Fatalf("true negation lost: %s: %+v", phrase, p)
		}
	}
}

func TestImperativesParticlesAndInlineRequests(t *testing.T) {
	names := []string{"吧啦", "说书人"}
	for _, prefix := range []string{"麻烦说说", "劳驾讲讲", "烦请看看", "查一下", "看一看", "说说"} {
		for _, tail := range []string{"吧。", "呢！", "呗，谢谢。", "呀，可以吗？", "。"} {
			q := prefix + names[0] + "的落地成本" + tail
			p := parseQuestion(q, names)
			if !p.natural || !slices.Equal(p.Subjects, names[:1]) || len(p.Constraints) != 0 {
				t.Fatalf("%s: %+v", q, p)
			}
		}
	}
	for _, verb := range requestVerbs() {
		for _, operator := range []string{verb + verb, verb + "一下", verb + "一" + verb} {
			q := fmt.Sprintf("%s与%s逐一%s生命点数。", names[0], names[1], operator)
			p := parseQuestion(q, names)
			if !p.natural || !slices.Equal(p.Subjects, names) || len(p.Constraints) != 0 {
				t.Fatalf("%s: %+v", q, p)
			}
		}
	}
	for _, copula := range []string{"各为", "各自为", "分别为", "各是"} {
		p := parseQuestion("吧啦和说书人的HP"+copula+"多少？", names)
		if !slices.Equal(p.Subjects, names) || len(p.Constraints) != 0 {
			t.Fatal(p)
		}
	}
	for _, modifier := range []string{"受说说效果影响", "吧状态下", "模组后", "明年", "至少120", "不能", "未解锁"} {
		p := parseQuestion("麻烦说说吧啦"+modifier+"的HP吧。", names)
		if !slices.Equal(p.Subjects, names[:1]) || len(p.Constraints) == 0 {
			t.Fatalf("real condition lost: %s: %+v", modifier, p)
		}
	}
	p := parseQuestion("吧啦和灰栎逐一说说血量吧？", names)
	if !slices.Equal(p.Subjects, []string{"吧啦", "灰栎"}) {
		t.Fatal("missing parallel subject lost", p)
	}
	for _, q := range []string{"吧啦 HP", "麻烦树 血量", "攻击间隔 1.0s"} {
		if parseQuestion(q, []string{"吧啦", "麻烦树"}).natural {
			t.Fatal("bare keywords became an imperative", q)
		}
	}
}
