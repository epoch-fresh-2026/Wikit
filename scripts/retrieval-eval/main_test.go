package main

import (
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

func TestStrictCoverageRequiresNamedMissingSubjects(t *testing.T) {
	legacy := true
	q := question{ExpectedSubjects: []string{"灰栎"}, ExpectedMissingSubjects: []string{"灰栎"}}
	m := measurement{CoverageCorrect: &legacy, RAG: &search.RAGContext{}}
	if ok, _, _ := strictCoverage(q, m); ok {
		t.Fatal("unverified refusal hid a dropped subject")
	}
	m.RAG.Plan.Subjects = []string{"灰栎"}
	if ok, _, _ := strictCoverage(q, m); ok {
		t.Fatal("subject name without a not_found disposition passed")
	}
	m.RAG.Coverage.MissingSubjects = []string{"灰栎"}
	m.RAG.Coverage.Subjects = []search.SubjectCoverage{{Subject: "灰栎", Status: "attribute_missing"}}
	if ok, _, _ := strictCoverage(q, m); ok {
		t.Fatal("wrong missing reason passed")
	}
	m.RAG.Coverage.Subjects[0].Status = "not_found"
	if ok, _, _ := strictCoverage(q, m); !ok {
		t.Fatal("explicit correctly missing subject failed")
	}
	q.ExpectedSubjects, q.ExpectedMissingSubjects = []string{}, []string{}
	if ok, _, _ := strictCoverage(q, m); ok {
		t.Fatal("invented subject passed an explicit empty annotation")
	}
	q.ExpectedSubjects, q.ExpectedMissingSubjects = nil, nil
	if ok, _, _ := strictCoverage(q, m); !ok {
		t.Fatal("legacy fixture changed")
	}
}
