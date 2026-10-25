package search

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type RequirementBinding struct {
	Subject string `json:"subject"`
	AttributeRequirement
}

// Consumers use these exact requirements, never the union of unrelated clauses.
func subjectRequirements(p questionSpec, subject string) []AttributeRequirement {
	if len(p.Bindings) == 0 {
		return requirements(p)
	}
	out := []AttributeRequirement{}
	for _, b := range p.Bindings {
		if b.Subject == subject {
			out = append(out, b.AttributeRequirement)
		}
	}
	return out
}

var bindingSeparator = regexp.MustCompile(`以及|还有|跟|和|与|及|、|，|,|；|;|。|？|\?`)
var coordinatedSubjectSlot = regexp.MustCompile(`^(?:以及|还有|跟|和|与|及|、)\s*zzbinding[0-9]+zz\s*`)

func bindQuestion(q string, known []string, p *questionSpec) {
	p.Bindings = []RequirementBinding{}
	if !p.natural || p.Scope != "named" {
		return
	}
	shared := func() {
		for _, subject := range p.Subjects {
			for _, r := range requirements(*p) {
				p.Bindings = append(p.Bindings, RequirementBinding{subject, r})
			}
		}
	}
	// Protect dynamic title/subject slots before interpreting conjunctions. Names
	// containing grammatical words must not become boundaries or attributes.
	names := slices.Clone(p.Subjects)
	slices.SortStableFunc(names, func(a, b string) int { return len(b) - len(a) })
	keys := map[string]string{}
	masked := q
	for i, name := range names {
		key := fmt.Sprintf("zzbinding%dzz", i)
		masked = protectTitleAlias(masked, name, key)
		keys[key] = name
	}
	type group struct {
		text      string
		subjects  []string
		attribute bool
	}
	groups := []group{{subjects: []string{}}}
	last := 0
	separators := bindingSeparator.FindAllStringIndex(masked, -1)
	separators = append(separators, []int{len(masked), len(masked)})
	for _, span := range separators {
		part := masked[last:span[0]]
		partSubjects := []string{}
		language := part
		for key, name := range keys {
			if strings.Contains(part, key) {
				partSubjects = append(partSubjects, name)
				language = strings.ReplaceAll(language, key, " ")
			}
		}
		current := &groups[len(groups)-1]
		if current.attribute && len(partSubjects) > 0 {
			groups = append(groups, group{subjects: []string{}})
			current = &groups[len(groups)-1]
		}
		current.text += part + masked[span[0]:span[1]]
		for _, name := range partSubjects {
			if !slices.Contains(current.subjects, name) {
				current.subjects = append(current.subjects, name)
			}
		}
		for _, entry := range attributeWords() {
			for _, alias := range entry.aliases {
				if containsLanguage(language, alias) {
					current.attribute = true
				}
			}
		}
		last = span[1]
	}
	// A fronted attribute followed by its subject is one request, not two
	// independent subject clauses.
	if len(groups) < 2 || len(groups[0].subjects) == 0 {
		shared()
		return
	}
	fallback := func() {
		if !slices.Contains(p.Constraints, "attribute_scope_unverified") {
			p.Constraints = append(p.Constraints, "attribute_scope_unverified")
		}
		shared()
	}
	bindings := []RequirementBinding{}
	constraints := []string{}
	allSubjects := []string{}
	for _, g := range groups {
		if !g.attribute || len(g.subjects) == 0 {
			fallback()
			return
		}
		text := g.text
		for key, name := range keys {
			at := strings.Index(text, key)
			if at < 0 || slices.Contains(known, name) {
				continue
			}
			after := strings.TrimSpace(text[at+len(key):])
			for {
				span := coordinatedSubjectSlot.FindStringIndex(after)
				if span == nil {
					break
				}
				after = strings.TrimSpace(after[span[1]:])
			}
			stages := stageExpression.FindStringIndex(after)
			if !strings.HasPrefix(after, "的") && !(len(stages) > 0 && stages[0] == 0) {
				// Residue in the legacy subject list is not itself proof of a
				// subject slot. Unknown names need explicit possession/staging.
				fallback()
				return
			}
		}
		firstSubject := len(text)
		for key := range keys {
			if at := strings.Index(text, key); at >= 0 && at < firstSubject {
				firstSubject = at
			}
		}
		// A stage before the first subject could scope the entire coordination.
		// Do not reinterpret it as a local modifier without a syntactic anchor.
		if stageExpression.MatchString(text[:firstSubject]) {
			fallback()
			return
		}
		for key, name := range keys {
			text = strings.ReplaceAll(text, key, name)
		}
		clause := parseUnboundQuestion(text+"？", known)
		if !clause.natural || clause.Scope != "named" || len(clause.Subjects) != len(g.subjects) {
			fallback()
			return
		}
		for _, subject := range clause.Subjects {
			if !slices.Contains(g.subjects, subject) {
				fallback()
				return
			}
			if !slices.Contains(allSubjects, subject) {
				allSubjects = append(allSubjects, subject)
			}
			for _, r := range requirements(clause) {
				b := RequirementBinding{subject, r}
				if !slices.ContainsFunc(bindings, func(x RequirementBinding) bool {
					return x.Subject == b.Subject && x.Attribute == b.Attribute && sameStage(x.Stage, b.Stage) && slices.Equal(x.Filters, b.Filters)
				}) {
					bindings = append(bindings, b)
				}
			}
		}
		for _, c := range clause.Constraints {
			if !slices.Contains(constraints, c) {
				constraints = append(constraints, c)
			}
		}
	}
	if len(allSubjects) != len(p.Subjects) || len(bindings) > 64 || slices.Contains(p.Constraints, "attribute_budget") || slices.Contains(p.Constraints, "subject_budget") {
		fallback()
		return
	}
	summary := []AttributeRequirement{}
	for _, b := range bindings {
		if !slices.ContainsFunc(summary, func(r AttributeRequirement) bool { return r.Attribute == b.Attribute }) {
			summary = append(summary, b.AttributeRequirement)
		}
	}
	if len(summary) > 8 {
		p.Constraints = append(p.Constraints, "attribute_budget")
		fallback()
		return
	}
	// Each original fragment was reparsed, so only now replace global stage and
	// scope warnings with the independently preserved local constraints.
	p.Bindings = bindings
	p.Requirements = summary
	p.Constraints = constraints
	p.boundClauses = true
}

func sameStage(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
