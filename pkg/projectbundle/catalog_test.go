package projectbundle

import (
	"bytes"
	"testing"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func TestCatalogRequiresOriginalSelectedReleaseAndPreservesOtherHistory(t *testing.T) {
	value := fixture(t, false)
	root, _ := value.Root()
	record := ReleaseRecord{ProjectID: root.ProjectID(), Version: root.Version(), CreatedAt: "2026-10-01T01:02:03.123456Z", CreatedByID: "owner", Title: "Original title"}
	if _, err := NewCatalog(value, []ReleaseRecord{}); err == nil {
		t.Fatal("fabricated missing release metadata")
	}
	catalog, err := NewCatalog(value, []ReleaseRecord{record})
	if err != nil {
		t.Fatal(err)
	}
	before, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(value, raw, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	records, err := decoded.SelectedReleases(value)
	if err != nil || len(records) != 1 || records[0] != record {
		t.Fatalf("catalog lost selected history: %v", err)
	}
	prior := record
	prior.Version = "0"
	current := record
	current.Title = "Renamed later"
	current.ArchivedAt = ptr(projectsnapshot.Timestamp("2026-10-02T00:00:00Z"))
	current.ArchivedMessage = "retired"
	changed, err := NewCatalog(value, []ReleaseRecord{current, prior})
	if err != nil {
		t.Fatal(err)
	}
	records, err = changed.SelectedReleases(value)
	if err != nil || len(records) != 1 || records[0].Version != record.Version {
		t.Fatalf("catalog-only release gained a body: %v", err)
	}
	after, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before[ManifestPath], after[ManifestPath]) {
		t.Fatal("catalog change entered frozen manifest")
	}
	// The old catalog stays detached when later values and caller buffers change.
	raw[0] = '!'
	*current.ArchivedAt = "changed"
	stable, err := EncodeCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCatalog(value, stable, 1<<20); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRejectsMissingCreatorWrongSourcesAndCoercion(t *testing.T) {
	value := fixture(t, false)
	root, _ := value.Root()
	original := ReleaseRecord{ProjectID: root.ProjectID(), Version: root.Version(), CreatedAt: "2026-10-01T00:00:00Z", CreatedByID: "owner"}
	for _, field := range []string{"actor", "time", "project", "version"} {
		t.Run(field, func(t *testing.T) {
			record := original
			switch field {
			case "actor":
				record.CreatedByID = "not-captured"
			case "time":
				record.CreatedAt = ""
			case "project":
				record.ProjectID = "absent"
			case "version":
				record.Version = ""
			}
			if _, err := NewCatalog(value, []ReleaseRecord{record}); err == nil {
				t.Fatal("accepted unavailable catalog identity")
			}
		})
	}
	catalog, err := NewCatalog(value, []ReleaseRecord{original})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCatalog(value, raw, 1); err == nil {
		t.Fatal("ignored catalog byte budget")
	}
	raw = bytes.Replace(raw, []byte(`"description": ""`), []byte(`"description": null`), 1)
	if _, err := DecodeCatalog(value, raw, 1<<20); err == nil {
		t.Fatal("accepted nullable coercion")
	}
	if _, err := NewCatalog(fixture(t, true), []ReleaseRecord{}); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRequiresSelectedSourceReleaseMetadata(t *testing.T) {
	rootValue, sourceValue := fixtureProject(t, false, "root"), fixtureProject(t, false, "source")
	rootID, _ := rootValue.Root()
	sourceID, _ := sourceValue.Root()
	root, _ := rootValue.Input(rootID)
	source, _ := sourceValue.Input(sourceID)
	root.Dependencies = []projectsnapshot.Identity{sourceID}
	document := root.Evidence.Descriptors().Document()
	document.ProjectModules[sourceID.ProjectID()] = source.Evidence.Descriptors().Module()
	descriptors, err := releasedescriptor.Decode(encodeJSON(t, document), rootID.ProjectID(), rootID.Version())
	if err != nil {
		t.Fatal(err)
	}
	sections, err := root.Evidence.CaptureSections()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := releasecontent.DecodeVerified(encodeJSON(t, receiptFile{Format: 1, Sections: sections}), rootID.ProjectID(), rootID.Version())
	if err != nil {
		t.Fatal(err)
	}
	root.Evidence, err = projectsnapshot.VerifiedEvidence(receipt, descriptors, []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := projectsnapshot.NewComplete(root, []projectsnapshot.ProjectInput{source})
	if err != nil {
		t.Fatal(err)
	}
	rootRecord := ReleaseRecord{ProjectID: "root", Version: "1", CreatedAt: "2026-10-02T00:00:00Z", CreatedByID: "owner"}
	if _, err := NewCatalog(graph, []ReleaseRecord{rootRecord}); err == nil {
		t.Fatal("accepted root-only catalog with a source release")
	}
	sourceRecord := ReleaseRecord{ProjectID: "source", Version: "1", CreatedAt: "2026-10-01T00:00:00Z", CreatedByID: "owner"}
	catalog, err := NewCatalog(graph, []ReleaseRecord{rootRecord, sourceRecord})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := catalog.SelectedReleases(graph)
	if err != nil || len(selected) != 2 || selected[1] != sourceRecord {
		t.Fatalf("source history lost: %v", err)
	}
	sourceRecord.Version = "2"
	if _, err := NewCatalog(graph, []ReleaseRecord{rootRecord, sourceRecord}); err == nil {
		t.Fatal("rebound source release metadata")
	}
}
