package search

import (
	"fmt"
	"slices"
	"testing"
)

func TestMedialRequestsAndDistributiveConnectors(t *testing.T) {
	for _, frame := range []string{"%s的HP你直接给我报一下。", "%s的HP，帮我简短报一下。", "%s的HP，你大致报个数就行。", "%s的HP，一人各一行列出。", "%s的HP，逐人一项说个数。"} {
		for _, name := range []string{"赤槐", "12F"} {
			q := fmt.Sprintf(frame, name)
			p := parseQuestion(q, []string{name, "简短", "你大致"})
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatal(q, p)
			}
		}
	}
	for _, q := range []string{"赤槐满生命有几多点？", "赤槐初登场的开销要多少？", "赤槐的生命封顶是多少？"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) != 0 {
			t.Fatal(q, p)
		}
	}
	for _, name := range []string{"简短", "你大致", "一人各一行", "逐人一项"} {
		for _, known := range [][]string{nil, {name}} {
			p := parseQuestion(name+"的生命封顶报给我。", known)
			if !slices.Equal(p.Subjects, []string{name}) || len(p.Constraints) != 0 {
				t.Fatal("real subject lost", name, p)
			}
		}
	}
	for _, q := range []string{"赤槐的HP受你直接效果影响，简短报一下。", "赤槐的HP你直接给我报一下至少100点。", "赤槐模组后的HP你直接给我报一下。", "赤槐满生命时的攻击力，你大致报一下。"} {
		p := parseQuestion(q, []string{"赤槐"})
		if !slices.Equal(p.Subjects, []string{"赤槐"}) || len(p.Constraints) == 0 {
			t.Fatal("condition lost", q, p)
		}
	}
}

func TestCompiledGrammarReturnsIndependentCopies(t *testing.T) {
	for _, table := range []func() []string{requestOperators, requestDistributionPhrases} {
		first := table()
		expected := slices.Clone(first)
		first[0] = "changed"
		if !slices.Equal(table(), expected) {
			t.Fatal("caller mutated compiled grammar")
		}
	}
}
