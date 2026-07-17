package generators

import "testing"

func TestSlug(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "semantic id", in: []string{"LA.M.1 Person"}, want: "la-m-1-person"},
		{name: "fallback skips empty", in: []string{"", "Birth Event"}, want: "birth-event"},
		{name: "resource fallback", in: []string{"", "   "}, want: "resource"},
		{name: "qname", in: []string{"crm:P1_is_identified_by"}, want: "crm-p1-is-identified-by"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slug(tt.in...); got != tt.want {
				t.Fatalf("Slug() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSlugAllocatorSuffixesCollisions(t *testing.T) {
	alloc := newSlugAllocator()

	got := []string{
		alloc.Next("Name"),
		alloc.Next("Name"),
		alloc.Next("name"),
	}
	want := []string{"name", "name-2", "name-3"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slug %d = %q, want %q", i, got[i], want[i])
		}
	}
}
