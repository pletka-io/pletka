package vocabservice

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeURIUpgradesGettyToHTTPS(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"http://vocab.getty.edu/aat/300010957", "https://vocab.getty.edu/aat/300010957"},
		{"https://vocab.getty.edu/aat/300010957", "https://vocab.getty.edu/aat/300010957"},
		{"http://vocab.getty.edu/page/aat/300010957", "https://vocab.getty.edu/aat/300010957"},
		{"  ", ""},
		{"not a url", "not a url"},
	} {
		if got := NormalizeURI(tc.in); got != tc.want {
			t.Errorf("NormalizeURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The service returns the ancestor chain as one comma-joined string, nearest
// ancestor first. Angle-bracketed guide terms are part of the chain and are
// kept verbatim.
func TestSplitParentString(t *testing.T) {
	const in = "koperlegering, <koper en koperlegering>, metaal"
	want := []string{"koperlegering", "<koper en koperlegering>", "metaal"}
	if diff := cmp.Diff(want, SplitParentString(in)); diff != "" {
		t.Errorf("SplitParentString mismatch (-want +got):\n%s", diff)
	}
	if got := SplitParentString("   "); got != nil {
		t.Errorf("SplitParentString(blank) = %v, want nil", got)
	}
}

func TestParentStringRefsLabelsEachAncestorInTheRequestedLanguage(t *testing.T) {
	refs := ParentStringRefs("koperlegering, metaal", "nl")
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].Label["nl"] != "koperlegering" {
		t.Errorf("refs[0].Label[nl] = %q, want %q", refs[0].Label["nl"], "koperlegering")
	}
	if refs[1].Label["nl"] != "metaal" {
		t.Errorf("refs[1].Label[nl] = %q, want %q", refs[1].Label["nl"], "metaal")
	}
	if refs[0].URI != "" || refs[0].ID != "" {
		t.Error("a parent-string ref carries no URI or ID of its own; the caller fills item 0 from the broader URI")
	}
	if got := ParentStringRefs("", "nl"); got != nil {
		t.Errorf("ParentStringRefs(blank) = %v, want nil", got)
	}
}
