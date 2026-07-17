package scope

import "testing"

func TestScopeURL(t *testing.T) {
	s := New("/projects/ABC")

	got := s.URL("/models/", "{id}", "form-schema")
	want := "/projects/ABC/models/{id}/form-schema"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestProjectEscapesProjectID(t *testing.T) {
	got := Project("A B").URL("models")
	want := "/projects/A%20B/models"
	if got != want {
		t.Fatalf("Project().URL() = %q, want %q", got, want)
	}
}
