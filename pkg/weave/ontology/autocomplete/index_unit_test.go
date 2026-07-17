package autocomplete

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// fakeIndexStore implements indexStore with canned data for unit tests.
type fakeIndexStore struct {
	classes    []*domain.OntologyClass
	properties []*domain.OntologyProperty
	relations  []*domain.OntologyRelationWithSource
	versions   map[string]*domain.OntologyVersion
	ontologies map[string]*domain.Ontology
}

func (f *fakeIndexStore) ListClassesByVersions(_ context.Context, _ []string) ([]*domain.OntologyClass, error) {
	return f.classes, nil
}

func (f *fakeIndexStore) ListPropertiesByVersions(_ context.Context, _ []string) ([]*domain.OntologyProperty, error) {
	return f.properties, nil
}

func (f *fakeIndexStore) ListRelationsByVersionsAndTypes(_ context.Context, _, _ []string) ([]*domain.OntologyRelationWithSource, error) {
	return f.relations, nil
}

func (f *fakeIndexStore) GetVersion(_ context.Context, id string) (*domain.OntologyVersion, error) {
	return f.versions[id], nil
}

func (f *fakeIndexStore) GetOntology(_ context.Context, id string) (*domain.Ontology, error) {
	return f.ontologies[id], nil
}

// TestBuildIndex_Ghost_And_PrimaryMetaCollapse exercises:
//
//	(a) when two versions define the same qname, the node is collapsed to one
//	    and the primary version's Meta wins;
//	(b) a relation whose target qname has no backing class/property row
//	    synthesises a ghost Node with empty edge slices.
func TestBuildIndex_Ghost_And_PrimaryMetaCollapse(t *testing.T) {
	const (
		primaryVersionID = "v-primary"
		secondVersionID  = "v-second"
		ontologyID       = "ont-1"
	)

	store := &fakeIndexStore{
		// "x:A" is declared in both versions — collapse to one Node.
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-a-primary",
				OntologyVersionID: primaryVersionID,
				Prefix:            "x",
				LocalName:         "A",
				URI:               "http://example.org/A",
				Qname:             "x:A",
				Label:             domain.Translations{"en": "Class A (primary)"},
			},
			{
				ID:                "cls-a-second",
				OntologyVersionID: secondVersionID,
				Prefix:            "x",
				LocalName:         "A",
				URI:               "http://example.org/A",
				Qname:             "x:A",
				Label:             domain.Translations{"en": "Class A (second)"},
			},
		},
		properties: nil,
		// A subclass_of relation targeting "x:DANGLING" — a qname that has no
		// class or property row. This should produce a ghost Node.
		relations: []*domain.OntologyRelationWithSource{
			{
				OntologyRelation: domain.OntologyRelation{
					SourceID:    "cls-a-primary",
					SourceKind:  "class",
					RelType:     "subclass_of",
					TargetQname: "x:DANGLING",
				},
				SourceQname: "x:A",
			},
		},
		versions: map[string]*domain.OntologyVersion{
			primaryVersionID: {ID: primaryVersionID, OntologyID: ontologyID, VersionString: "1.0"},
			secondVersionID:  {ID: secondVersionID, OntologyID: ontologyID, VersionString: "0.9"},
		},
		ontologies: map[string]*domain.Ontology{
			ontologyID: {ID: ontologyID, Prefix: "x"},
		},
	}

	versionIDs := []string{primaryVersionID, secondVersionID}
	key := keyForRequest(versionIDs, primaryVersionID, "live")
	idx, err := buildIndex(context.Background(), store, nil, key, versionIDs, primaryVersionID, testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	// (a) Only one Node for "x:A".
	nodeA := idx.ByQname["x:A"]
	if nodeA == nil {
		t.Fatal("x:A not found in index")
	}
	if nodeA.Ghost {
		t.Fatal("x:A should not be a ghost")
	}
	// Primary version's Meta should be on the collapsed node.
	if nodeA.Meta.VersionID != primaryVersionID {
		t.Errorf("x:A Meta.VersionID = %q; want %q", nodeA.Meta.VersionID, primaryVersionID)
	}
	if nodeA.Meta.VersionString != "1.0" {
		t.Errorf("x:A Meta.VersionString = %q; want \"1.0\"", nodeA.Meta.VersionString)
	}

	// (b) x:DANGLING is a ghost with empty edge slices.
	ghost := idx.ByQname["x:DANGLING"]
	if ghost == nil {
		t.Fatal("x:DANGLING ghost node not found in index")
	}
	if !ghost.Ghost {
		t.Fatal("x:DANGLING should be Ghost=true")
	}
	if len(ghost.Subclasses) != 1 {
		t.Errorf("ghost.Subclasses len = %d; want 1 (x:A)", len(ghost.Subclasses))
	}
	if ghost.Subclasses[0] != nodeA {
		t.Error("ghost.Subclasses[0] should point back to x:A")
	}
	// Ghost has no superclasses of its own.
	if len(ghost.Superclasses) != 0 {
		t.Errorf("ghost.Superclasses len = %d; want 0", len(ghost.Superclasses))
	}
	// nodeA's Superclasses should point at the ghost.
	if len(nodeA.Superclasses) != 1 || nodeA.Superclasses[0] != ghost {
		t.Errorf("x:A Superclasses should be [x:DANGLING], got len %d", len(nodeA.Superclasses))
	}
}

func TestBuildIndex_NormalizesFullURIRangeTargets(t *testing.T) {
	const versionID = "v-primary"

	store := &fakeIndexStore{
		properties: []*domain.OntologyProperty{
			{
				ID:                "prop-value",
				OntologyVersionID: versionID,
				Prefix:            "x",
				LocalName:         "hasValue",
				URI:               "http://example.org/hasValue",
				Qname:             "x:hasValue",
				PropertyType:      "DatatypeProperty",
				Label:             domain.Translations{"en": "has value"},
			},
		},
		relations: []*domain.OntologyRelationWithSource{
			{
				OntologyRelation: domain.OntologyRelation{
					SourceID:    "prop-value",
					SourceKind:  "property",
					RelType:     "range",
					TargetQname: "http://www.w3.org/2001/XMLSchema#string",
				},
				SourceQname: "x:hasValue",
			},
		},
		versions:   map[string]*domain.OntologyVersion{versionID: {ID: versionID}},
		ontologies: map[string]*domain.Ontology{},
	}
	nsMap := func(context.Context) ([]NamespaceBinding, error) {
		return []NamespaceBinding{
			{Prefix: "xsd", Namespace: "http://www.w3.org/2001/XMLSchema#"},
		}, nil
	}

	versionIDs := []string{versionID}
	key := keyForRequest(versionIDs, versionID, "live")
	idx, err := buildIndex(context.Background(), store, nsMap, key, versionIDs, versionID, testLogger())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if idx.ByQname["http://www.w3.org/2001/XMLSchema#string"] != nil {
		t.Fatal("full URI range target should not be kept as an index node")
	}
	ghost := idx.ByQname["xsd:string"]
	if ghost == nil {
		t.Fatal("xsd:string ghost node not found")
	}
	if !ghost.Ghost {
		t.Fatal("xsd:string should be a ghost literal target")
	}
	prop := idx.ByQname["x:hasValue"]
	if prop == nil || len(prop.Ranges) != 1 || prop.Ranges[0] != ghost {
		t.Fatalf("property range was not wired to normalized ghost node")
	}
}

// TestBuildIndex_DetectsSubclassCycle verifies that detectCycles emits a Warn
// log when the subclass_of graph contains a loop. The build must still succeed
// (no error) because cycles are diagnostic-only.
func TestBuildIndex_DetectsSubclassCycle(t *testing.T) {
	const (
		versionID  = "v-cycle"
		ontologyID = "ont-cycle"
	)

	store := &fakeIndexStore{
		// x:A and x:B both exist as real class rows.
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-a",
				OntologyVersionID: versionID,
				Prefix:            "x",
				LocalName:         "A",
				URI:               "http://example.org/A",
				Qname:             "x:A",
				Label:             domain.Translations{"en": "Class A"},
			},
			{
				ID:                "cls-b",
				OntologyVersionID: versionID,
				Prefix:            "x",
				LocalName:         "B",
				URI:               "http://example.org/B",
				Qname:             "x:B",
				Label:             domain.Translations{"en": "Class B"},
			},
		},
		properties: nil,
		// x:A subclass_of x:B  AND  x:B subclass_of x:A  — a 2-node cycle.
		relations: []*domain.OntologyRelationWithSource{
			{
				OntologyRelation: domain.OntologyRelation{
					SourceID:    "cls-a",
					SourceKind:  "class",
					RelType:     "subclass_of",
					TargetQname: "x:B",
				},
				SourceQname: "x:A",
			},
			{
				OntologyRelation: domain.OntologyRelation{
					SourceID:    "cls-b",
					SourceKind:  "class",
					RelType:     "subclass_of",
					TargetQname: "x:A",
				},
				SourceQname: "x:B",
			},
		},
		versions: map[string]*domain.OntologyVersion{
			versionID: {ID: versionID, OntologyID: ontologyID, VersionString: "1.0"},
		},
		ontologies: map[string]*domain.Ontology{
			ontologyID: {ID: ontologyID, Prefix: "x"},
		},
	}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	versionIDs := []string{versionID}
	key := keyForRequest(versionIDs, versionID, "live")
	idx, err := buildIndex(context.Background(), store, nil, key, versionIDs, versionID, log)
	if err != nil {
		t.Fatalf("buildIndex: unexpected error: %v", err)
	}

	// Nodes must be present despite the cycle.
	if idx.ByQname["x:A"] == nil || idx.ByQname["x:B"] == nil {
		t.Fatal("expected x:A and x:B in index")
	}

	// The Warn log must contain the cycle-detected message.
	const wantMsg = "autocomplete index: subclass cycle detected"
	if !bytes.Contains(buf.Bytes(), []byte(wantMsg)) {
		t.Errorf("expected log to contain %q\ngot: %s", wantMsg, buf.String())
	}
}

// testLogger returns slog.Default() — enough for unit tests.
func testLogger() *slog.Logger { return slog.Default() }
