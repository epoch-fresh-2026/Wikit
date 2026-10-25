package search

import "testing"

// Use representative existing regression cases, independent of external corpora.
func BenchmarkQuestionPlanning(b *testing.B) {
	names := []string{"赤槐", "蓝杉", "和风"}
	for name, question := range map[string]string{
		"single":     "请问赤槐的血量大概是多少呀？",
		"comparison": "赤槐和蓝杉分别谁在精英四满级时攻击更高？",
		"unknown":    "赤槐和灰栎的攻击间隔是多少？",
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				parseQuestion(question, names)
			}
		})
	}
}
