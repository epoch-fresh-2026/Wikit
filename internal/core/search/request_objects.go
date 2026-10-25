package search

import "strings"

// Recognize a nominalized request only with its complete operation and an
// attribute-bearing object. Completion words inside the object are untouched.
func nominalRequest(value string) (string, string, bool) {
	if !strings.HasPrefix(value, "完成") {
		return "", "", false
	}
	for _, action := range requestVerbs() {
		if strings.HasSuffix(value, "的"+action) {
			object := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "完成"), "的"+action))
			if hasRequestAttribute(object) {
				// Nominal request objects may omit the subject's possessive
				// particle. Make that boundary explicit only for a bare noun.
				first := len(object)
				for _, entry := range attributeWords() {
					for _, alias := range entry.aliases {
						if at := strings.Index(object, alias); at >= 0 && at < first {
							first = at
						}
					}
				}
				prefix := strings.TrimSpace(object[:first])
				if !strings.Contains(prefix, "的") && subjectNoun(prefix) {
					object = prefix + "的" + object[first:]
				}
				return action, object, true
			}
		}
	}
	return "", "", false
}

// Work on a whole punctuation-delimited request, before subject conjunctions
// can separate a disposal prefix from its trailing output operation.
func normalizeRequestObject(part string) string {
	value := trimLanguageTail(part, requestTails())
	if rest, ok := consumeRequestPrefixes(value); ok {
		value = rest
	}
	if action, object, ok := nominalRequest(value); ok {
		return action + " " + object
	}
	if strings.HasPrefix(value, "把") || strings.HasPrefix(value, "将") {
		body := strings.TrimSpace(value[len("把"):])
		for _, action := range compiledRequestOperators {
			if !strings.HasSuffix(body, action) {
				continue
			}
			object := strings.TrimSpace(strings.TrimSuffix(body, action))
			object = trimLanguageTail(object, compiledRequestPrefixes)
			if hasRequestAttribute(object) {
				part = action + " " + object
				break
			}
		}
	}
	// Compact possession is accepted only at the subject slot of a request,
	// immediately before a known attribute or stage, not inside arbitrary names.
	for from := 0; from < len(part); {
		relative := strings.Index(part[from:], "其")
		if relative < 0 {
			break
		}
		at := from + relative
		from = at + len("其")
		prefix := part[:at]
		if boundaries := bindingSeparator.FindAllStringIndex(prefix, -1); len(boundaries) > 0 {
			prefix = prefix[boundaries[len(boundaries)-1][1]:]
		}
		before := cleanRequestClause(prefix)
		before = stripDisposalRequest(before, true)
		after := strings.TrimSpace(part[at+len("其"):])
		attribute := false
		for _, entry := range attributeWords() {
			for _, alias := range entry.aliases {
				attribute = attribute || languageAt(after, 0, alias)
			}
		}
		stage := stageExpression.FindStringIndex(after)
		if strings.TrimSpace(before) == "" && (attribute || len(stage) > 0 && stage[0] == 0) {
			part = part[:at] + "其的" + part[at+len("其"):]
			from += len("的")
		}
	}
	return part
}
