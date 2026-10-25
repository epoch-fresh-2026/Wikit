package search

import (
	"slices"
	"testing"
)

func TestRequestVocabularyAppendDoesNotMutateSharedStorage(t *testing.T) {
	for name, vocabulary := range map[string]func() []string{
		"operators": requestOperators, "distribution": requestDistributionPhrases,
		"courtesy": courtesyPrefixes, "benefactive": benefactivePrefixes,
		"organization": requestOrganizationWords, "tails": requestTails,
		"verbs": requestVerbs, "addressees": requestAddressees,
		"adverbs": requestAdverbs, "polar": polarForms,
	} {
		t.Run(name, func(t *testing.T) {
			original := slices.Clone(vocabulary())
			derived := append(vocabulary(), "local extension")
			derived[0] = "local override"
			if !slices.Equal(vocabulary(), original) {
				t.Fatal("extending a vocabulary modified the shared words")
			}
		})
	}
}
