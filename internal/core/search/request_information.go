package search

import "strings"

// Generic information nouns can frame the object of a request, but are not
// globally removable: "攻击数据提升20%" still carries a condition.
func stripAttributeInformationNouns(value string) string {
	tails := append(append(requestTails(), "是什么", "是多少", "如何", "怎样", "怎么样"), requestOrganizationWords()...)
	for _, entry := range attributeWords() {
		for _, attribute := range entry.aliases {
			for from := 0; from < len(value); {
				rel := strings.Index(value[from:], attribute)
				if rel < 0 {
					break
				}
				at := from + rel + len(attribute)
				if !languageAt(value, from+rel, attribute) {
					from = at
					continue
				}
				tail := strings.TrimSpace(value[at:])
				for _, noun := range []string{"资料", "信息", "数据", "情况"} {
					if !strings.HasPrefix(tail, noun) {
						continue
					}
					rest := strings.TrimSpace(strings.TrimPrefix(tail, noun))
					if trimLanguageTail(rest, tails) == "" {
						return value[:at] + " " + rest
					}
				}
				from = at
			}
		}
	}
	return value
}
