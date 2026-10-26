package search

import (
	"strings"
	"unicode"
)

// Unknown names need a grammatical subject slot, not an authorized title match.
// Possession identifies that slot even after a leading request. A compact
// argument in a leading request can also precede a parallel subject clause.
func consumeUnknownSubject(language, original string, protected map[string]string, collection bool) (string, string) {
	for key := range protected {
		if strings.Contains(language, key) {
			return "", language
		}
	}
	language = strings.TrimSpace(language)
	for at, r := range language {
		if r != '的' {
			continue
		}
		candidate := strings.TrimSpace(language[:at])
		after := strings.TrimSpace(language[at+len("的"):])
		if !subjectNoun(candidate) {
			continue
		}
		for _, entry := range attributeWords() {
			for _, alias := range entry.aliases {
				if languageAt(after, 0, alias) {
					return candidate, language[at:]
				}
			}
		}
	}
	if !collection && isRequestClause(original) && containsRequestOperator(original) && subjectNoun(language) {
		for _, entry := range attributeWords() {
			for _, alias := range entry.aliases {
				if containsLanguage(language, alias) {
					return "", language
				}
			}
		}
		return language, ""
	}
	return "", language
}

func subjectNoun(value string) bool {
	if _, reference := referenceCardinality(value); reference {
		return true
	}
	if value == "" || len([]rune(value)) > 64 || isModifierClause(value) || isRequestClause(value) {
		return false
	}
	grammar := append(requestOrganizationWords(), "样本", "干员", "对象", "页面", "哪些", "这些", "所有", "全部", "各", "都", "谁")
	if strings.TrimSpace(stripLanguage(value, grammar)) == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	for _, prefix := range []string{"受", "受到", "加上", "降低", "至少", "大于", "小于", "不含"} {
		if strings.HasPrefix(value, prefix) {
			return false
		}
	}
	for _, suffix := range []string{"后", "前", "期间", "影响", "效果", "加成", "修正", "版本"} {
		if strings.HasSuffix(value, suffix) {
			return false
		}
	}
	return true
}
