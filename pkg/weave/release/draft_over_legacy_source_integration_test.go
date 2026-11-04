//go:build integration

package release

import (
	"errors"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/projectontology"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

// A consumer's draft must plan its ontology closure over a source whose release
// carries only legacy (contract 2) evidence. The source's retained selection
// pins the draft to the same published version; an unpublished selection is a
// validation error for the editor, never missing release content.
func TestDraftPublicationsOverLegacySource(t *testing.T) {
	ctx, p, svc := seedLegacyRebuild(t, true)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild); err != nil {
		t.Fatal(err)
	}
	// The root draft keeps one version, in its link and in its field path, so
	// that its own selection is publishable without a version conflict.
	mustExec(t, p, `DELETE FROM weave_project_ontology_versions WHERE project_id='legacy-root' AND ontology_version_id='legacy-crm-6.2'`)
	mustExec(t, p, `UPDATE weave_fields SET path_elements='[{"position":0,"type":"class","uri":"https://example.org/crm/E1","prefix":"crm","local_name":"E1"},{"position":1,"type":"literal","uri":"rdfs:Literal","local_name":"Literal","prefix":"rdfs","datatype":"rdf:literal"}]' WHERE id='legacy-field'`)
	draft := func() ([]ontologyartifact.Artifact, error) {
		var out []ontologyartifact.Artifact
		err := readsession.NewFactory(p).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, auth.Draft(), "legacy-root", func(s *readsession.Session) error {
			var err error
			out, err = s.DraftPublications(ctx)
			return err
		})
		return out, err
	}

	_, err = draft()
	var invalid *projectontology.InvalidError
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "legacy-crm-7.1.3") || !strings.Contains(err.Error(), "legacy-source@0.1.0") {
		t.Fatalf("want the source's unpublished selection as a validation error, got %v", err)
	}

	ref, err := ontologybody.NewReference("legacy-crm", "legacy-crm-7.1.3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ontologyrelease.PublishCurrent(ctx, p, ref, ontologyrelease.Options{}); err != nil {
		t.Fatal(err)
	}
	artifacts, err := draft()
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, a := range artifacts {
		d, err := a.Decode()
		if err != nil {
			t.Fatal(err)
		}
		versions = append(versions, d.VersionID)
	}
	if len(versions) != 1 || versions[0] != "legacy-crm-7.1.3" {
		t.Fatalf("draft closure %v, want the version the legacy source retained", versions)
	}
}
