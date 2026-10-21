package access

import (
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"testing"
)

func TestCapabilityMatrix(t *testing.T) {
	for _, tc := range []struct {
		role, status, visibility, pageStatus string
		want                                 []string
	}{
		{"", "", "public", "published", []string{"read"}},
		{"member", "active", "public", "published", []string{"read", "propose", "discuss"}},
		{"moderator", "active", "public", "published", []string{"read", "propose", "discuss", "merge"}},
		{"admin", "active", "restricted", "published", capabilities()},
		{"moderator", "active", "restricted", "published", nil},
		{"member", "disabled", "public", "published", nil},
		{"moderator", "active", "public", "hidden", nil},
	} {
		a := identity.Actor{Record: identity.Record{Account: identity.Account{Role: tc.role, Status: tc.status}}}
		if tc.role != "" {
			a.ID = "user"
		}
		got := effective(a, Policy{Visibility: tc.visibility, Status: tc.pageStatus}, nil)
		if len(got) != len(tc.want) {
			t.Fatalf("%s/%s/%s: %v", tc.role, tc.visibility, tc.pageStatus, got)
		}
		for _, c := range tc.want {
			if !got[c] {
				t.Fatalf("missing %s", c)
			}
		}
	}
}
func TestManageDoesNotImplyReview(t *testing.T) {
	a := identity.Actor{Record: identity.Record{Account: identity.Account{ID: "manager", Role: "member", Status: "active"}}}
	p := Policy{Visibility: "restricted", Status: "hidden", Grants: []Grant{{"group", "group", []string{"read", "manage"}}}}
	caps := effective(a, p, []Group{{ID: "group", Members: []string{"manager"}}})
	if !caps["read"] || !caps["manage"] || caps["merge"] {
		t.Fatal(caps)
	}
}
