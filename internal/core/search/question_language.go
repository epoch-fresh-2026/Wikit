package search

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A beneficiary inside a clause is grammatical only immediately before a
// request (with optional organization words), not inside an unknown effect.
func stripInlineBenefactives(value string) string {
	prefixes, operators := benefactivePrefixes(), compiledRequestOperators
	var out strings.Builder
	for at := 0; at < len(value); {
		consumed := 0
		for _, prefix := range prefixes {
			if !strings.HasPrefix(value[at:], prefix) {
				continue
			}
			rest := strings.TrimSpace(value[at+len(prefix):])
			for {
				before := rest
				for _, mode := range requestOrganizationWords() {
					if strings.HasPrefix(rest, mode) {
						rest = strings.TrimSpace(strings.TrimPrefix(rest, mode))
						break
					}
				}
				if before == rest {
					break
				}
			}
			for _, operator := range operators {
				if strings.HasPrefix(rest, operator) {
					consumed = len(prefix)
					break
				}
			}
			if consumed > 0 {
				break
			}
		}
		if consumed > 0 {
			out.WriteByte(' ')
			at += consumed
		} else {
			r, size := utf8.DecodeRuneInString(value[at:])
			out.WriteRune(r)
			at += size
		}
	}
	return out.String()
}

// Distribution describes output only immediately before a request action.
// Consume it before removing the action, while its grammatical role is known.
// Possessive subject slots and effects containing the same phrase stay intact.
func stripRequestDistribution(value string) string {
	phrases := compiledRequestDistribution
	operators := compiledRequestOperators
	var out strings.Builder
	for at := 0; at < len(value); {
		consumed := 0
		for _, phrase := range phrases {
			if !strings.HasPrefix(value[at:], phrase) {
				continue
			}
			rest := strings.TrimSpace(value[at+len(phrase):])
			for _, operator := range operators {
				if strings.HasPrefix(rest, operator) {
					consumed = len(phrase)
					break
				}
			}
		}
		if consumed > 0 {
			out.WriteByte(' ')
			at += consumed
		} else {
			r, size := utf8.DecodeRuneInString(value[at:])
			out.WriteRune(r)
			at += size
		}
	}
	return out.String()
}

func cleanRequestClause(value string) string {
	value = stripAttributeRequestPrefixes(value)
	if rest, ok := consumeRequestPrefixes(value); ok {
		value = rest
	}
	value = stripRequestDistribution(stripInlineBenefactives(value))
	value = stripAttributeDemonstratives(stripInlineRequests(stripRequestFrame(value)))
	// The caller has established question intent for the whole coordination.
	// An attribute continuation need not repeat the request action.
	return stripAttributeInformationNouns(value)
}

// Consume a sequence of grammatical roles only when it leads to a complete
// request action or a disposal clause containing one. A possessive slot ends
// the sequence so unknown names are not silently treated as request grammar.
func consumeRequestPrefixes(value string) (string, bool) {
	value = strings.TrimSpace(value)
	rest := value
	for {
		before := rest
		for _, prefix := range compiledRequestPrefixes {
			if strings.HasPrefix(rest, prefix) {
				after := strings.TrimSpace(strings.TrimPrefix(rest, prefix))
				if after != "" && !strings.HasPrefix(after, "的") {
					rest = after
					break
				}
			}
		}
		if before == rest {
			break
		}
	}
	if rest != value {
		if _, _, ok := nominalRequest(rest); ok {
			return rest, true
		}
		for _, operator := range compiledRequestOperators {
			if strings.HasPrefix(rest, operator) {
				return rest, true
			}
		}
		if (strings.HasPrefix(rest, "把") || strings.HasPrefix(rest, "将")) && containsRequestOperator(rest) {
			return rest, true
		}
	}
	return value, false
}

// Medial request prefixes are grammatical only immediately after an attribute
// slot and before a confirmed request action. Leave the action and any actual
// conditions after it for normal parsing; never remove arbitrary middle words.
func stripAttributeRequestPrefixes(value string) string {
	vocabulary := attributeWords()
	var out strings.Builder
	for at := 0; at < len(value); {
		consumed := 0
		attribute := ""
		for _, entry := range vocabulary {
			for _, alias := range entry.aliases {
				if !languageAt(value, at, alias) {
					continue
				}
				tail := strings.TrimSpace(value[at+len(alias):])
				rest, ok := consumeRequestPrefixes(tail)
				if !ok || strings.HasPrefix(rest, "把") || strings.HasPrefix(rest, "将") {
					continue
				}
				if length := len(alias) + len(value[at+len(alias):]) - len(rest); length > consumed {
					consumed = length
					attribute = alias
				}
			}
		}
		if consumed > 0 {
			// Keep the recognized attribute itself; only its following prefix
			// chain is replaced. The longest matching span wins.
			out.WriteString(attribute)
			out.WriteByte(' ')
			at += consumed
		} else {
			r, size := utf8.DecodeRuneInString(value[at:])
			out.WriteRune(r)
			at += size
		}
	}
	return out.String()
}

func containsRequestOperator(value string) bool {
	for _, operator := range compiledRequestOperators {
		if strings.Contains(value, operator) {
			return true
		}
	}
	return false
}

// Request clauses cannot introduce subjects merely because their remaining
// imperative vocabulary is unknown. A separate parallel name still can.
func isRequestClause(value string) bool {
	if _, ok := consumeRequestPrefixes(value); ok {
		return true
	}
	value = strings.TrimSpace(value)
	for _, prefix := range compiledRequestStarts {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// Names are protected. Only a request with a recognizable attribute can use
// 把/将 as a disposal marker; qualifiers and changes to the value remain intact.
func stripDisposalRequest(value string, attributeKnown bool) string {
	value = strings.TrimSpace(value)
	if !attributeKnown {
		return value
	}
	for _, marker := range []string{"把", "将"} {
		if strings.HasPrefix(value, marker) {
			return strings.TrimSpace(strings.TrimPrefix(value, marker))
		}
	}
	return value
}

// A readable title can itself be an imperative. Preserve it in subject slots,
// but do not reinterpret an attribute-following request as a title occurrence.
func protectTitleAlias(value, name, key string) string {
	var out strings.Builder
	from := 0
	for {
		relative := strings.Index(value[from:], name)
		if relative < 0 {
			out.WriteString(value[from:])
			return out.String()
		}
		at := from + relative
		out.WriteString(value[from:at])
		before, after := value[:at], value[at+len(name):]
		// A named effect/provider inside 受...影响 is a condition, not a new
		// requested comparison subject. Keep the text for constraint parsing.
		modifierPosition := (strings.HasSuffix(strings.TrimSpace(before), "受") || strings.HasSuffix(strings.TrimSpace(before), "受到")) &&
			strings.Contains(splitSubjectClauses(after)[0], "影响")
		operatorPosition := modifierPosition
		trimmedBefore := strings.TrimSpace(before)
		last, _ := utf8.DecodeLastRuneInString(trimmedBefore)
		benefactiveAtBoundary := slices.Contains(benefactivePrefixes(), name) &&
			(trimmedBefore == "" || strings.ContainsRune("，,；;。.!！?？", last))
		requestContinuation := false
		for _, operator := range compiledRequestOperators {
			if len(operator) > len(name) && strings.HasPrefix(operator, name) && strings.HasPrefix(name+after, operator) {
				requestContinuation = true
			}
		}
		rest, requestPrefix := consumeRequestPrefixes(name + after)
		requestPrefix = requestPrefix && len(name+after)-len(rest) >= len(name)
		if (slices.Contains(compiledRequestNames, name) || requestPrefix) && !strings.HasPrefix(strings.TrimSpace(after), "的") {
			followsAttribute := requestContinuation || benefactiveAtBoundary || trimLanguageTail(after, requestTails()) == "" || containsRequestOperator(after)
			precedesAttribute := strings.TrimSpace(before) == "" || isRequestClause(before)
			for _, entry := range attributeWords() {
				for _, alias := range entry.aliases {
					if followsAttribute && containsLanguage(before, alias) || precedesAttribute && containsLanguage(after, alias) {
						operatorPosition = true
					}
				}
			}
		}
		if operatorPosition {
			out.WriteString(name)
		} else {
			out.WriteString(" " + key + " ")
		}
		from = at + len(name)
	}
}

func trimLanguageTail(value string, tails []string) string {
	trim := func(v string) string {
		return strings.TrimFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	}
	value = trim(value)
	for {
		before := value
		for _, suffix := range tails {
			if strings.HasSuffix(value, suffix) {
				value = trim(strings.TrimSuffix(value, suffix))
				break
			}
		}
		if value == before {
			return value
		}
	}
}

// Productive A-not-A forms ask polarity; they do not assert a negative fact.

func neutralizePolarQuestions(value string) string {
	forms := polarForms()
	var out strings.Builder
	var previous rune
	for at := 0; at < len(value); {
		matched := false
		if !strings.ContainsRune("不没无未", previous) {
			for _, form := range forms {
				if strings.HasPrefix(value[at:], form) {
					out.WriteByte(' ')
					at += len(form)
					previous = ' '
					matched = true
					break
				}
			}
		}
		if !matched {
			r, size := utf8.DecodeRuneInString(value[at:])
			out.WriteRune(r)
			previous = r
			at += size
		}
	}
	return out.String()
}

func hasQuestionIntent(value string, names []string) bool {
	// A title containing a courtesy word must not turn bare keywords into a request.
	for _, name := range names {
		if name != "" {
			value = protectTitleAlias(value, name, "zznamezz")
		}
	}
	if strings.ContainsAny(value, "？?吗哪多少几什么是否谁") || strings.Contains(value, "比较") || strings.Contains(value, "对比") || strings.Contains(value, "请") || strings.Contains(value, "帮") || neutralizePolarQuestions(value) != value || stripInlineRequests(value) != value {
		return true
	}
	// A request can follow the subject/attribute in a separate clause.
	for _, clause := range splitSubjectClauses(value) {
		clause = strings.TrimSpace(clause)
		if isRequestClause(clause) {
			return true
		}
		for _, operator := range compiledRequestOperators {
			if strings.HasPrefix(clause, operator) || strings.HasSuffix(trimLanguageTail(clause, requestTails()), operator) {
				return true
			}
		}
	}
	return false
}

// Names have already been protected. Courtesy and modal particles frame a
// clause; they are not globally removable single-character qualifiers.
func stripRequestFrame(value string) string {
	if rest, ok := consumeRequestPrefixes(value); ok {
		value = rest
	}
	tails := compiledRequestEnds
	value = trimLanguageTail(value, tails)
	// Consume a sequence of prefixes, with each iteration strictly shortening it.
	for {
		before := value
		for _, prefix := range courtesyPrefixes() {
			if strings.HasPrefix(value, prefix) {
				value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
				for _, pronoun := range []string{"您", "你"} {
					value = strings.TrimSpace(strings.TrimPrefix(value, pronoun))
				}
				break
			}
		}
		if value == before {
			break
		}
	}
	for _, operator := range compiledRequestOperators {
		if strings.HasPrefix(value, operator) {
			if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(value, operator)), "的") {
				return value
			}
			return strings.TrimSpace(strings.TrimPrefix(value, operator))
		}
	}
	for _, verb := range requestVerbs() {
		if strings.HasPrefix(value, verb) {
			value = strings.TrimPrefix(value, verb)
			value = strings.TrimPrefix(value, verb)
			value = strings.TrimPrefix(value, "一"+verb)
			break
		}
	}
	return value
}

// A sentence-medial request operator can precede an attribute slot, including
// after a subject or distributive marker. Unknown surrounding effects remain.
func stripInlineRequests(value string) string {
	return stripBeforeAttributes(value, compiledRequestOperators)
}

// Deictic classifier phrases label an attribute slot, not an arbitrary effect.
func stripAttributeDemonstratives(value string) string {
	return stripBeforeAttributes(value, phraseAliases([]string{"这", "那", "该", "此"}, []string{"条", "种", "项", "个"}))
}

func stripBeforeAttributes(value string, operators []string) string {
	attributes := compiledAttributeAliases
	var out strings.Builder
	for at := 0; at < len(value); {
		consumed := 0
		for _, operator := range operators {
			if !strings.HasPrefix(value[at:], operator) {
				continue
			}
			rest := strings.TrimSpace(value[at+len(operator):])
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "的"))
			for _, attribute := range attributes {
				if languageAt(rest, 0, attribute) {
					consumed = len(operator)
					break
				}
			}
		}
		if consumed > 0 {
			out.WriteByte(' ')
			at += consumed
		} else {
			r, size := utf8.DecodeRuneInString(value[at:])
			out.WriteRune(r)
			at += size
		}
	}
	return out.String()
}
