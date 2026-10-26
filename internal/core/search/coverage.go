package search

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type SubjectCoverage struct {
	Subject string   `json:"subject"`
	Status  string   `json:"status"`
	PageIDs []string `json:"page_ids"`
}
type Coverage struct {
	Requirements           []RequirementCoverage `json:"requirements"`
	Status                 string                `json:"status"`
	Subjects               []SubjectCoverage     `json:"subjects"`
	MissingSubjects        []string              `json:"missing_subjects"`
	UnsupportedConstraints []string              `json:"unsupported_constraints"`
	Truncated              bool                  `json:"truncated"`
}

type RequirementCoverage struct {
	Stage       *string  `json:"stage"`
	Subject     string   `json:"subject"`
	Attribute   string   `json:"attribute"`
	Status      string   `json:"status"`
	PageIDs     []string `json:"page_ids"`
	CitationIDs []string `json:"citation_ids"`
}

var numericCell = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]+)?|[—–-]+)$`)

// Stage columns and values must be associated in the same source section.
// A header alone is not evidence that a particular attribute has a value.
func stageSupport(blocks []TextBlock, attribute, requested string) string {
	columns := []string{}
	section := ""
	sawHeader, seenStage := false, false
	for _, b := range blocks {
		id := ""
		if b.SectionID != nil {
			id = *b.SectionID
		}
		if id != section {
			columns = []string{}
			section = id
		}
		if matches := stageExpression.FindAllStringSubmatch(b.Text, -1); len(matches) > 0 {
			columns = []string{}
			sawHeader = true
			for _, m := range matches {
				n, ok := smallNumber(m[1])
				if ok {
					columns = append(columns, "精英"+strconv.Itoa(n))
				}
			}
			if slices.Contains(columns, requested) {
				seenStage = true
			}
		}
		at := strings.Index(b.Text, attribute)
		if at < 0 || len(columns) == 0 || !slices.Contains(columns, requested) {
			continue
		}
		values := []string{}
		for _, token := range strings.Fields(b.Text[at+len(attribute):]) {
			if !numericCell.MatchString(token) {
				break
			}
			values = append(values, token)
		}
		if len(values) < len(columns) {
			continue
		}
		matched := false
		for i, column := range columns {
			if column != requested {
				continue
			}
			matched = true
			if strings.ContainsAny(values[i], "—–-") {
				return "stage_unsupported"
			}
		}
		if matched {
			return "covered"
		}
	}
	if sawHeader && !seenStage {
		return "stage_unsupported"
	}
	return "unverified"
}

func attributePresent(p Passage, attribute string) bool {
	// A label or an empty section is a locator, not evidence of its value.
	if strings.Trim(strings.TrimSpace(p.Text), ":： ") == attribute || strings.TrimSpace(p.Text) == "" {
		return false
	}
	if slices.Contains([]string{"生命上限", "攻击", "阻挡数", "初始部署费用", "再部署时间", "攻击间隔"}, attribute) {
		at := strings.Index(p.Text, attribute)
		if at < 0 {
			return false
		}
		tail := strings.TrimSpace(p.Text[at+len(attribute):])
		return len(tail) > 0 && tail[0] >= '0' && tail[0] <= '9'
	}
	return strings.Contains(p.Text, attribute) || slices.Contains(p.SectionPath, attribute)
}

func assessCoverage(plan questionSpec, items []Hit, evidence []Evidence, blocks map[string][]TextBlock, truncated bool) Coverage {
	c := Coverage{Status: "insufficient", Requirements: []RequirementCoverage{}, Subjects: []SubjectCoverage{}, MissingSubjects: []string{}, UnsupportedConstraints: slices.Clone(plan.Constraints), Truncated: truncated}
	if c.UnsupportedConstraints == nil {
		c.UnsupportedConstraints = []string{}
	}
	names := slices.Clone(plan.Subjects)
	if plan.Scope != "named" {
		names = []string{}
		for _, h := range items {
			alias := titleAlias(h.Title)
			if !slices.Contains(names, alias) {
				names = append(names, alias)
			}
		}
	}
	if len(names) > 8 {
		names = names[:8]
		c.Truncated = true
	}
	covered := 0
	for _, name := range names {
		s := SubjectCoverage{Subject: name, Status: "not_found", PageIDs: []string{}}
		subjectCovered := 0
		requested := subjectRequirements(plan, name)
		if len(requested) == 0 {
			s.Status = "unverified"
			c.UnsupportedConstraints = append(c.UnsupportedConstraints, "unbound_subject:"+name)
		}
		for _, requirement := range requested {
			row := RequirementCoverage{Subject: name, Attribute: requirement.Attribute, Stage: requirement.Stage, Status: "not_found", PageIDs: []string{}, CitationIDs: []string{}}
			for _, h := range items {
				if titleAlias(h.Title) != name && h.Title != name {
					continue
				}
				if !slices.Contains(s.PageIDs, h.PageID) {
					s.PageIDs = append(s.PageIDs, h.PageID)
				}
				row.PageIDs = append(row.PageIDs, h.PageID)
				if row.Status == "not_found" {
					row.Status = "attribute_missing"
				}
				for _, e := range evidence {
					if e.PageID != h.PageID || e.RevisionID != h.RevisionID || !attributePresent(e.Passage, requirement.Attribute) {
						continue
					}
					matches := true
					for _, filter := range requirement.Filters {
						if !strings.Contains(e.Passage.Text, filter) {
							matches = false
						}
					}
					if !matches {
						continue
					}
					state := "covered"
					if !plan.natural {
						state = "unverified"
					} else if requirement.Stage != nil {
						state = stageSupport(blocks[h.PageID], requirement.Attribute, *requirement.Stage)
					}
					if row.Status != "covered" {
						row.Status = state
					}
					if state == "covered" && e.CitationID != "" && !slices.Contains(row.CitationIDs, e.CitationID) {
						row.CitationIDs = append(row.CitationIDs, e.CitationID)
					}
				}
			}
			if unresolvedReference(plan, name) || row.Status == "not_found" && plan.identityUnverified[name] {
				row.Status = "unverified"
			}
			if row.Status == "covered" {
				subjectCovered++
				covered++
			}
			if row.Status == "stage_unsupported" && requirement.Stage != nil {
				constraint := name + ":" + *requirement.Stage
				if !slices.Contains(c.UnsupportedConstraints, constraint) {
					c.UnsupportedConstraints = append(c.UnsupportedConstraints, constraint)
				}
			}
			if s.Status == "not_found" || row.Status == "stage_unsupported" || row.Status == "unverified" {
				s.Status = row.Status
			}
			c.Requirements = append(c.Requirements, row)
		}
		if subjectCovered > 0 && subjectCovered == len(requested) {
			s.Status = "covered"
		} else if subjectCovered > 0 {
			s.Status = "partial"
		}
		if s.Status == "not_found" {
			c.MissingSubjects = append(c.MissingSubjects, name)
		}
		c.Subjects = append(c.Subjects, s)
	}
	if plan.expectedCount > 0 && len(names) < plan.expectedCount {
		c.Truncated = true
	}
	if covered > 0 {
		c.Status = "partial"
	}
	if covered == len(c.Requirements) && covered > 0 {
		c.Status = "complete"
	}
	if !plan.natural || (c.Status == "complete" && (c.Truncated || len(c.UnsupportedConstraints) > 0)) {
		c.Status = "unverified"
	}
	return c
}
