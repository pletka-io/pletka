// Package testsnapshot constructs validated synthetic values for consumer tests.
package testsnapshot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

const fixtureOwner = "owner"
const fixtureOntology = "fixture"
const fixtureVersion = "fixture-1"

// Source serves one fully validated immutable value in tests.
type Source struct {
	Value projectsnapshot.CompleteProject
}

// Entity selects without any live capability.
func (s Source) Entity(_ context.Context, _ auth.ReadScope, _ string, kind, id string) (projectsnapshot.EntityFragment, error) {
	return s.Value.Entity(kind, id)
}
func ptr[T any](v T) *T { return &v }
func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func hash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// GeneratorValue builds a draft model/collection fixture with actual evidence.
func GeneratorValue(t *testing.T, project domain.Project, model domain.Model, view domain.ModelView, collection domain.Collection, fields []domain.ResolvedField) projectsnapshot.CompleteProject {
	t.Helper()
	c := projectsnapshot.Content{}
	rv := reflect.ValueOf(&c).Elem()
	for i := range rv.NumField() {
		rv.Field(i).Set(reflect.MakeSlice(rv.Field(i).Type(), 0, 0))
	}
	c.Projects = []projectsnapshot.ProjectRecord{{ID: project.ID, OwnerID: fixtureOwner, Namespace: ptr(project.Namespace), UIName: encode(t, project.UIName), SystemName: ptr(project.SystemName), EffectiveConceptNamespace: ptr("https://concepts.example/"), EnforceConceptLists: ptr(false)}}
	c.Namespaces = []projectsnapshot.NamespaceRecord{{ID: "ns", ReleaseProjectID: project.ID, Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Weight: 10}}
	if model.ID != "" {
		c.Models = append(c.Models, projectsnapshot.ModelRecord{ID: model.ID, ProjectID: project.ID, UIName: encode(t, model.UIName), SystemName: ptr(model.SystemName), OntologyScope: encode(t, model.OntologyScope), ModelType: "auxiliary"})
	}
	if collection.ID != "" {
		c.Collections = append(c.Collections, projectsnapshot.CollectionRecord{ID: collection.ID, ProjectID: project.ID, UIName: encode(t, collection.UIName), SystemName: ptr(collection.SystemName), OntologyScope: encode(t, collection.OntologyScope)})
	}
	seen := map[string]bool{}
	add := func(f domain.ResolvedField, kind, id, category, coll string) {
		if !seen[f.ID] {
			seen[f.ID] = true
			c.Fields = append(c.Fields, projectsnapshot.FieldRecord{ID: f.ID, ProjectID: project.ID, SemanticID: ptr(f.SemanticID), SystemName: ptr(f.SystemName), ExpectedValueType: ptr(f.ExpectedValueType), PathElements: encode(t, f.PathElements), SubfieldPaths: encode(t, f.SubfieldPaths)})
		}
		o := projectsnapshot.OverrideRecord{ID: int64(len(c.Overrides) + 1), FieldID: f.ID, ProjectID: project.ID, EntityType: kind, EntityID: id, Position: checkedInt32(t, f.Position), DisplayName: encode(t, f.DisplayName), Description: encode(t, f.Description), CollectionName: encode(t, f.CollectionName), SetValue: ptr(f.SetValue), IsRequired: ptr(f.IsRequired), MinOccurs: ptr(checkedInt32(t, f.MinOccurs)), IsHidden: ptr(f.IsHidden), CollectionOrder: checkedInt32(t, f.CollectionOrder)}
		if category != "" {
			o.CategoryID = ptr(category)
		}
		if coll != "" {
			o.PartOfCollectionID = ptr(coll)
		}
		if f.MaxOccurs != nil {
			o.MaxOccurs = ptr(checkedInt32(t, *f.MaxOccurs))
		}
		c.Overrides = append(c.Overrides, o)
	}
	for _, cat := range view.Categories {
		if cat.ID != "" {
			c.Categories = append(c.Categories, projectsnapshot.CategoryRecord{ID: cat.ID, ProjectID: project.ID, UIName: encode(t, cat.Name), CanonicalOrder: checkedInt32(t, cat.Position)})
		}
		for _, coll := range cat.Collections {
			for _, f := range coll.Fields {
				collID := coll.ID
				if collID == "__direct__" {
					collID = ""
				}
				add(f, "model", model.ID, cat.ID, collID)
			}
		}
	}
	for _, f := range fields {
		add(f, "collection", collection.ID, "", "")
	}
	for _, f := range c.Fields {
		c.Overrides = append(c.Overrides, projectsnapshot.OverrideRecord{ID: int64(len(c.Overrides) + 1), ProjectID: project.ID, FieldID: f.ID})
	}
	return Complete(t, c)
}

// Complete supplies synthetic verified term evidence and the explicit empty sections.
func Complete(t *testing.T, c projectsnapshot.Content) projectsnapshot.CompleteProject {
	t.Helper()
	id, err := projectsnapshot.Draft(c.Projects[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	elems, err := projectsnapshot.SemanticElements(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := ontologyevidence.InputReferences(elems)
	if err != nil {
		t.Fatal(err)
	}
	classes, properties := []ontologyartifact.Term{}, []ontologyartifact.Term{}
	refs := make([]ontologyartifact.TermResolution, 0, len(inputs))
	seen := map[string]bool{}
	for _, in := range inputs {
		uri := in.Reference.URI
		prefix, local := in.Reference.Prefix, in.Reference.LocalName
		if local == "" {
			prefix, local, _ = strings.Cut(uri, ":")
		}
		if !strings.Contains(uri, "://") {
			uri = "http://www.cidoc-crm.org/cidoc-crm/" + local
		}
		term := ontologyartifact.Term{URI: uri, Prefix: prefix, LocalName: local}
		k := in.Reference.Kind + uri
		if !seen[k] {
			seen[k] = true
			if in.Reference.Kind == "property" {
				properties = append(properties, term)
			} else {
				classes = append(classes, term)
			}
		}
		refs = append(refs, ontologyartifact.TermResolution{EntityKind: in.EntityKind, EntityID: in.EntityID, Location: in.Location, Reference: ontologyartifact.TermReference(in.Reference), OntologyID: fixtureOntology, VersionID: fixtureVersion, URI: uri})
	}
	slices.SortFunc(classes, func(a, b ontologyartifact.Term) int { return strings.Compare(a.URI, b.URI) })
	slices.SortFunc(properties, func(a, b ontologyartifact.Term) int { return strings.Compare(a.URI, b.URI) })
	body := string(encode(t, map[string]any{"format_version": 1, "ontology": map[string]any{"id": fixtureOntology, "prefix": "crm"}, "version": map[string]any{"id": fixtureVersion, "ontology_id": fixtureOntology, "version_string": "1"}, "classes": classes, "properties": properties, "relations": []any{}, "namespace_bindings": []any{}}))
	document := ontologyartifact.Document{FormatVersion: 1, OntologyID: fixtureOntology, VersionID: fixtureVersion, BodyJSON: body, BodyDigest: hash(body), Pins: []ontologyartifact.Pin{}, Requirements: []ontologyartifact.Requirement{}}
	raw := string(encode(t, document))
	artifact := ontologyartifact.Artifact{JSON: raw, Digest: hash(raw)}
	for i := range refs {
		refs[i].Digest = artifact.Digest
	}
	descriptors, err := releasedescriptor.Decode(encode(t, releasedescriptor.Document{Format: 1, ProjectID: id.ProjectID(), Actors: map[string]releasedescriptor.Actor{fixtureOwner: {ActorID: fixtureOwner}}, ProjectModules: map[string]string{id.ProjectID(): "module/test"}, OntologyModules: map[string]string{fixtureVersion: "ontology/test"}}), id.ProjectID(), "")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := projectsnapshot.DraftEvidence(id, descriptors, []ontologyartifact.ProjectArtifact{{Artifact: artifact}}, refs)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	v := reflect.ValueOf(c)
	for i := range v.NumField() {
		counts[v.Type().Field(i).Tag.Get("json")] = int64(v.Field(i).Len())
	}
	complete, err := projectsnapshot.NewComplete(projectsnapshot.ProjectInput{Content: c, Counts: counts, Evidence: evidence, Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return complete
}

// Project returns the retained test project.
func (s Source) Project(context.Context, auth.ReadScope, string) (projectsnapshot.CompleteProject, error) {
	return s.Value, nil
}

func checkedInt32(t *testing.T, n int) int32 {
	t.Helper()
	if n < math.MinInt32 || n > math.MaxInt32 {
		t.Fatalf("fixture integer out of range: %d", n)
	}
	return int32(n) //nolint:gosec // explicit MinInt32/MaxInt32 bounds above
}
