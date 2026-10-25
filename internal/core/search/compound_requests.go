package search

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var requestSequenceBoundary = regexp.MustCompile(`[，,；;。]`)

func requestCompletionComplements() []string { return []string{"完", "好"} }

// Normalize explicit request objects, compact references and completion chains
// before binding subjects. Protected titles and unknown conditions are retained;
// normalization never generates facts.
func normalizeCompoundRequests(q string, known []string) string {
	if strings.ContainsAny(q, "\"“”") || strings.Contains(q, "技能") {
		return q
	}
	names := slices.Clone(known)
	slices.SortStableFunc(names, func(a, b string) int { return len(b) - len(a) })
	masked := q
	keys := map[string]string{}
	for i, name := range names {
		if name != "" {
			key := fmt.Sprintf("zzcompound%dzz", i)
			masked = protectTitleAlias(masked, name, key)
			keys[key] = name
		}
	}
	spans := requestSequenceBoundary.FindAllStringIndex(masked, -1)
	parts := requestSequenceBoundary.Split(masked, -1)
	changed := false
	for i, part := range parts {
		parts[i] = normalizeRequestObject(part)
		changed = changed || parts[i] != part
	}
	for i := 0; i+1 < len(parts); i++ {
		next := strings.TrimSpace(parts[i+1])
		if rest, ok := consumeRequestPrefixes(next); ok {
			next = rest
		}
		if !slices.ContainsFunc(compiledRequestOperators, func(action string) bool {
			return strings.HasPrefix(next, action) && !strings.HasPrefix(strings.TrimSpace(next[len(action):]), "的")
		}) {
			continue
		}
		value := strings.TrimSpace(parts[i])
		// A completed fronted request may scope its own following temporal
		// boundary. Only remove that boundary when another request follows.
		front := value
		if rest, ok := consumeRequestPrefixes(front); ok {
			front = rest
		}
		for _, verb := range requestVerbs() {
			for _, complement := range requestCompletionComplements() {
				action := verb + complement
				if !strings.HasPrefix(front, action) {
					continue
				}
				for _, boundary := range []string{"之后", "以后", "后"} {
					if !strings.HasSuffix(front, boundary) {
						continue
					}
					object := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(front, action), boundary))
					if strings.HasPrefix(object, "的") || !hasRequestAttribute(object) {
						continue
					}
					parts[i] = verb + " " + object
					changed = true
					break
				}
			}
		}
		for _, action := range compiledRequestOperators {
			matched := false
			for _, completion := range []string{"之后", "以后", "后"} {
				if !strings.HasSuffix(value, action+completion) {
					continue
				}
				before := strings.TrimSpace(strings.TrimSuffix(value, action+completion))
				before = stripRequestFrame(before)
				if !strings.HasPrefix(before, "把") && !strings.HasPrefix(before, "将") {
					continue
				}
				object := strings.TrimSpace(before[len("把"):])
				if !hasRequestAttribute(object) {
					continue
				}
				parts[i] = action + " " + object
				changed, matched = true, true
				break
			}
			if matched {
				break
			}
		}
	}
	if !changed {
		return q
	}
	var out strings.Builder
	for i, part := range parts {
		out.WriteString(part)
		if i < len(spans) {
			out.WriteString(masked[spans[i][0]:spans[i][1]])
		}
	}
	resolved := out.String()
	for key, name := range keys {
		resolved = strings.ReplaceAll(resolved, key, name)
	}
	return resolved
}

func hasRequestAttribute(object string) bool {
	for _, entry := range attributeWords() {
		for _, alias := range entry.aliases {
			if containsLanguage(object, alias) {
				return true
			}
		}
	}
	return false
}
