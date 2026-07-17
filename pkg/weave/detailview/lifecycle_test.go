package detailview

import "testing"

func TestLifecycleCaps(t *testing.T) {
	const base = "/projects/P/models/M"
	cases := []struct {
		name          string
		editable      bool
		inUse         bool
		activeVersion string
		deprecated    bool
		wantDel       string
		wantDeprecate string
		wantActivate  string
	}{
		{"not editable", false, false, "", false, "", "", ""},
		{"pinned version", true, false, "v1", false, "", "", ""},
		{"editable active", true, false, "", false, base, base + "/deprecate", ""},
		{"editable deprecated", true, false, "", true, base, "", base + "/activate"},
		{"in use hides delete", true, true, "", false, "", base + "/deprecate", ""},
		{"in use + deprecated: activate only", true, true, "", true, "", "", base + "/activate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			del, dep, act := lifecycleCaps(tc.editable, tc.inUse, tc.activeVersion, base, tc.deprecated)
			if del != tc.wantDel || dep != tc.wantDeprecate || act != tc.wantActivate {
				t.Errorf("lifecycleCaps(%v,%q,_,%v) = (%q,%q,%q), want (%q,%q,%q)",
					tc.editable, tc.activeVersion, tc.deprecated,
					del, dep, act, tc.wantDel, tc.wantDeprecate, tc.wantActivate)
			}
		})
	}
}
