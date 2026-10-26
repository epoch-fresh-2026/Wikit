package search

import (
	"slices"
	"testing"
)

func TestReferenceBindings(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"青柏的获得方式和她的攻击间隔分别是什么？", []string{"青柏/获得方式", "青柏/攻击间隔"}},
		{"青柏的获得方式和他的攻击间隔分别是什么？", []string{"青柏/获得方式", "青柏/攻击间隔"}},
		{"未知树的获得方式和它的攻击间隔分别是什么？", []string{"未知树/获得方式", "未知树/攻击间隔"}},
		{"青柏和白榆的获得方式及她们的攻击间隔是什么？", []string{"青柏/获得方式", "白榆/获得方式", "青柏/攻击间隔", "白榆/攻击间隔"}},
		{"青柏和白榆的获得方式及前者的攻击间隔是什么？", []string{"青柏/获得方式", "白榆/获得方式", "青柏/攻击间隔"}},
		{"青柏和白榆的获得方式及后者的攻击间隔是什么？", []string{"青柏/获得方式", "白榆/获得方式", "白榆/攻击间隔"}},
		{"她山的获得方式和她的攻击间隔是什么？", []string{"她山/获得方式", "她山/攻击间隔"}},
	} {
		t.Run(tc.q, func(t *testing.T) {
			for _, known := range [][]string{nil, {"青柏", "白榆", "她山"}} {
				p := parseQuestion(tc.q, known)
				got := []string{}
				for _, b := range p.Bindings {
					got = append(got, b.Subject+"/"+b.Attribute)
				}
				slices.Sort(got)
				slices.Sort(tc.want)
				if !slices.Equal(got, tc.want) || len(p.Constraints) > 0 {
					t.Fatalf("got=%v plan=%+v", got, p)
				}
			}
		})
	}
}

func TestReferenceAmbiguityIsNotMissingIdentity(t *testing.T) {
	for _, q := range []string{"她的攻击间隔是多少？", "她的攻击间隔和青柏的获得方式是什么？", "青柏和白榆的获得方式及她的攻击间隔是什么？", "青柏的获得方式及后者的攻击间隔是什么？"} {
		p := parseQuestion(q, []string{"青柏", "白榆"})
		c := assessCoverage(p, nil, nil, nil, false)
		found := false
		for _, row := range c.Requirements {
			if unresolvedReference(p, row.Subject) {
				found = true
				if row.Status != "unverified" || slices.Contains(c.MissingSubjects, row.Subject) {
					t.Fatal(q, c)
				}
			}
		}
		if !found || c.Status == "complete" {
			t.Fatal(q, p, c)
		}
	}
}

func TestUnverifiedIdentityCoverage(t *testing.T) {
	p := parseQuestion("青柏的攻击间隔是多少？", nil)
	p.identityUnverified = map[string]bool{"青柏": true}
	c := assessCoverage(p, nil, nil, nil, true)
	if c.Requirements[0].Status != "unverified" || len(c.MissingSubjects) > 0 {
		t.Fatal(c)
	}
	c = assessCoverage(p, []Hit{{PageID: "1", Title: "青柏"}}, nil, nil, false)
	if c.Requirements[0].Status != "attribute_missing" || len(c.MissingSubjects) > 0 {
		t.Fatal(c)
	}
}

func TestReferenceKeepsLocalStagesAndConditions(t *testing.T) {
	p := parseQuestion("青柏精英一的生命上限和她精英二的攻击是多少？", []string{"青柏"})
	if len(p.Bindings) != 2 || len(p.Constraints) > 0 {
		t.Fatal(p)
	}
	for _, b := range p.Bindings {
		want := "精英1"
		if b.Attribute == "攻击" {
			want = "精英2"
		}
		if b.Subject != "青柏" || b.Stage == nil || *b.Stage != want {
			t.Fatal(p)
		}
	}
	p = parseQuestion("青柏的获得方式和她受到攻击提升30%后的攻击是多少？", []string{"青柏"})
	if len(p.Constraints) == 0 {
		t.Fatal("reference rewriting dropped conditions", p)
	}
}
