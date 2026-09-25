package vocabservice

import "testing"

func TestConceptID(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"http://vocab.getty.edu/aat/300010957", "300010957"},
		{"https://vocab.getty.edu/aat/300010957/", "300010957"},
		{"  ", ""},
		{"", ""},
	} {
		if got := conceptID(tc.in); got != tc.want {
			t.Errorf("conceptID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
