package cmd

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestOntologyDraftDocumentRejectsAmbiguousInput(t *testing.T) {
	for _, input := range []string{`{"unexpected":true}`, `{} {}`, `{`, `{"ontology":{"is_active":true}}`, `{"primary":{"version_id":"release"}}`} {
		cmd := &cobra.Command{}
		cmd.SetIn(strings.NewReader(input))
		if _, err := readOntologyDraftDocument(cmd, "-"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"format_version":1,"primary":{"filename":"main.rdf","content":"unfinished RDF"}}`))
	document, err := readOntologyDraftDocument(cmd, "-")
	if err != nil || document.Primary.Content != "unfinished RDF" {
		t.Fatalf("document: %+v %v", document, err)
	}
}

func TestOntologyDraftCommandRequiresExplicitInputs(t *testing.T) {
	for _, args := range [][]string{{"start", "owner"}, {"start", "owner", "--document", "-", "--from-release", "v1"}, {"save", "owner", "draft", "--document", "-"}, {"publish", "owner", "draft", "--version", "v1"}, {"publish", "owner", "draft", "--revision", "1"}, {"show"}} {
		cmd := newWeaveOntologyDraftCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted incomplete command %v", args)
		}
	}
}
