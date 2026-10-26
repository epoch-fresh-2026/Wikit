package search

import (
	"slices"
	"strings"
	"testing"
)

// Compare the optimized scanner to the original Unicode matching contract,
// including mixed scripts, identifier boundaries and overlapping occurrences.
func TestLanguageMatchingPreservesUnicodeAndBoundaries(t *testing.T) {
	words := append(slices.Clone(compiledAttributeAliases), "", "HP", "平A", "Σ", "σ", "ς", "K", "K", "s", "ſ", "alpha", "a-b", "?", "é")
	for _, word := range words {
		for _, value := range []string{word, "x" + word, word + "1", "中" + word + "文", "(" + word + ")", "x" + word + " " + word, strings.ToLower(word), strings.ToUpper(word), "未知内容", ""} {
			wantContains := false
			for at := range value {
				want := originalLanguageAt(value, at, word)
				wantContains = wantContains || want
				if got := languageAt(value, at, word); got != want {
					t.Fatalf("languageAt(%q, %d, %q) = %v; want %v", value, at, word, got, want)
				}
			}
			if got := containsLanguage(value, word); got != wantContains {
				t.Fatalf("containsLanguage(%q, %q) = %v; want %v", value, word, got, wantContains)
			}
		}
	}
}

func originalLanguageAt(value string, at int, word string) bool {
	if word == "" || at+len(word) > len(value) || !strings.EqualFold(value[at:at+len(word)], word) {
		return false
	}
	ascii := true
	for _, r := range word {
		ascii = ascii && r <= 127
	}
	left := at == 0 || !asciiIdentifier(value[at-1])
	right := at+len(word) == len(value) || !asciiIdentifier(value[at+len(word)])
	if ascii {
		return left && right
	}
	return (!asciiIdentifier(word[0]) || left) && (!asciiIdentifier(word[len(word)-1]) || right)
}
