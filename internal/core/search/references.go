package search

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var referenceSlot = regexp.MustCompile(`zzreference[0-9]+zz`)
var demonstrativeReference = regexp.MustCompile(`^(?:这|那|该|此|上述|前述|以上)(?:([零〇一二两三四五六七八九十0-9]*)(?:位|名|个)(?:干员|主体|对象|页面|人物|角色)?|(些)(?:干员|主体|对象|页面|人物|角色)?|(?:干员|主体|对象|页面|人物|角色))$`)
var countedReference = regexp.MustCompile(`^([零〇一二两三四五六七八九十0-9]+)者$`)
var classifiedReference = regexp.MustCompile(`^([零〇一二两三四五六七八九十0-9]+)(?:位|名|个)(?:干员|主体|对象|页面|人物|角色)(?:各自)?$`)
var personalCountReference = regexp.MustCompile(`^(?:这|那|该|此|上述|前述|以上)([零〇一二两三四五六七八九十0-9]+)人(?:各自)?$`)

// Positive counts are exact cardinalities; -1 is an open plural reference.
// Zero denotes an identifiable reference whose cardinality is unsupported.
func referenceCardinality(name string) (int, bool) {
	switch name {
	case "他", "她", "它", "其", "前者", "后者":
		return 1, true
	case "他们", "她们", "它们":
		return -1, true
	}
	if m := countedReference.FindStringSubmatch(name); m != nil {
		return boundedReferenceCount(m[1]), true
	}
	if m := classifiedReference.FindStringSubmatch(name); m != nil {
		return boundedReferenceCount(m[1]), true
	}
	if m := personalCountReference.FindStringSubmatch(name); m != nil {
		return boundedReferenceCount(m[1]), true
	}
	m := demonstrativeReference.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	if m[2] != "" {
		return -1, true
	}
	if m[1] == "" {
		return 1, true
	}
	return boundedReferenceCount(m[1]), true
}

func boundedReferenceCount(value string) int {
	n, ok := smallNumber(strings.ReplaceAll(value, "两", "二"))
	if !ok || n < 1 || n > 8 {
		return 0
	}
	return n
}

// Resolve only explicit subject slots already recognized by the planner. The
// antecedents come from this question, never document text or inferred gender.
func resolveReferences(q string, p questionSpec, known []string) (string, []string) {
	if !p.natural || p.Scope != "named" {
		return q, nil
	}
	names := slices.Clone(p.Subjects)
	slices.SortStableFunc(names, func(a, b string) int { return len(b) - len(a) })
	masked := q
	keys := map[string]string{}
	for i, name := range names {
		key := fmt.Sprintf("zzreference%dzz", i)
		masked = protectTitleAlias(masked, name, key)
		keys[key] = name
	}
	antecedents, unresolved := []string{}, []string{}
	resolved := referenceSlot.ReplaceAllStringFunc(masked, func(key string) string {
		name := keys[key]
		count, reference := referenceCardinality(name)
		if !reference || slices.Contains(known, name) {
			if !slices.Contains(antecedents, name) {
				antecedents = append(antecedents, name)
			}
			return name
		}
		target := ""
		switch name {
		case "前者", "后者":
			if len(antecedents) == 2 {
				i := 0
				if name == "后者" {
					i = 1
				}
				target = antecedents[i]
			}
		default:
			if count > 0 && len(antecedents) == count || count == -1 && len(antecedents) > 1 {
				target = strings.Join(antecedents, "和")
			}
		}
		if target == "" {
			if !slices.Contains(unresolved, name) {
				unresolved = append(unresolved, name)
			}
			return name
		}
		return target
	})
	// Masking inserts whitespace; do not reparse unrelated questions merely for
	// that normalization, since title-sensitive legacy grammar must stay intact.
	if len(unresolved) == 0 && !slices.ContainsFunc(p.Subjects, func(n string) bool {
		_, reference := referenceCardinality(n)
		return reference && !slices.Contains(known, n)
	}) {
		return q, nil
	}
	return resolved, unresolved
}

func unresolvedReference(p questionSpec, subject string) bool {
	return slices.Contains(p.Constraints, "unresolved_reference:"+subject)
}
