package search

import (
	"slices"
	"strings"
)

type AttributeRequirement struct {
	Attribute string   `json:"attribute"`
	Stage     *string  `json:"stage"`
	Filters   []string `json:"filters"`
}

func requirements(p questionSpec) []AttributeRequirement {
	if len(p.Requirements) > 0 {
		return p.Requirements
	}
	return []AttributeRequirement{{p.Attribute, p.Stage, p.Filters}}
}

func requirementFocus(r AttributeRequirement) string {
	a := r.Attribute
	switch a {
	case "优先攻击空中单位":
		a = "对空"
	case "阻挡数":
		a = "阻挡"
	}
	return strings.TrimSpace(a + " " + strings.Join(r.Filters, " "))
}

// Longest non-overlapping vocabulary spans distinguish an attribute from a
// substring of another attribute. Only coordinated spans add requirements;
// conditional mentions remain subject to the existing constraint parser.
func planRequirements(q string, p *questionSpec) {
	p.Requirements = []AttributeRequirement{{p.Attribute, p.Stage, slices.Clone(p.Filters)}}
	for _, name := range p.Subjects {
		q = protectTitleAlias(q, name, strings.Repeat("_", len(name)))
	}
	type span struct {
		start, end int
		attribute  string
	}
	spans := []span{}
	healthAliases, redeployment := fullHealthAliases(), redeploymentActions()
	for _, entry := range attributeWords() {
		for _, alias := range entry.aliases {
			if slices.Contains(healthAliases, alias) && p.Attribute != "生命上限" {
				continue
			}
			if slices.Contains(redeployment, alias) && p.Attribute != "再部署时间" {
				continue
			}
			for at := range q {
				if languageAt(q, at, alias) {
					spans = append(spans, span{at, at + len(alias), entry.name})
				}
			}
		}
	}
	slices.SortStableFunc(spans, func(a, b span) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return b.end - a.end
	})
	selected := []span{}
	for _, s := range spans {
		if len(selected) > 0 && s.start < selected[len(selected)-1].end {
			continue
		}
		selected = append(selected, s)
	}
	for i, s := range selected {
		coordinated := false
		for _, j := range []int{i - 1, i + 1} {
			if j < 0 || j >= len(selected) {
				continue
			}
			a, b := s, selected[j]
			if a.start > b.start {
				a, b = b, a
			}
			between := q[a.end:b.start]
			if strings.ContainsAny(between, "和与及、，,；;/&+") || strings.Contains(between, "还有") || strings.Contains(between, "跟") {
				coordinated = true
			}
		}
		if !coordinated {
			continue
		}
		a := s.attribute
		if a == "空中单位" {
			a = "对空"
			if p.Attribute == "优先攻击空中单位" {
				a = p.Attribute
			}
		}
		if slices.ContainsFunc(p.Requirements, func(r AttributeRequirement) bool { return r.Attribute == a }) {
			continue
		}
		if len(p.Requirements) == 8 {
			p.Constraints = append(p.Constraints, "attribute_budget")
			continue
		}
		p.Requirements = append(p.Requirements, AttributeRequirement{a, p.Stage, []string{}})
	}
	if len(p.Requirements) > 1 {
		// Multiple possessive clauses may assign different attributes to different
		// subjects. Until that binding is proven, a cross-product cannot be complete.
		bound := 0
		for _, clause := range splitSubjectClauses(q) {
			if strings.Contains(strings.ReplaceAll(clause, " ", ""), "_的") {
				bound++
			}
		}
		if bound > 1 {
			p.Constraints = append(p.Constraints, "attribute_scope_unverified")
		}
	}
}
