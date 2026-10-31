package projectbundle

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func ptr[T any](v T) *T { return &v }
func encodeJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func fixture(t *testing.T, draft bool) projectsnapshot.CompleteProject {
	t.Helper()
	return fixtureProject(t, draft, "P/../not-a-path")
}

func fixtureProject(t *testing.T, draft bool, project string) projectsnapshot.CompleteProject {
	t.Helper()
	id, err := projectsnapshot.Release(project, "1")
	if draft {
		id, err = projectsnapshot.Draft(project)
	}
	if err != nil {
		t.Fatal(err)
	}
	c := projectsnapshot.Content{}
	v := reflect.ValueOf(&c).Elem()
	for i := range v.NumField() {
		v.Field(i).Set(reflect.MakeSlice(v.Field(i).Type(), 0, 0))
	}
	c.Projects = []projectsnapshot.ProjectRecord{{ID: id.ProjectID(), VersionNumber: id.Version(), OwnerID: "owner", EffectiveConceptNamespace: ptr("https://example.test/"), EnforceConceptLists: ptr(false)}}
	c.Fields = []projectsnapshot.FieldRecord{
		{ID: "B", ProjectID: id.ProjectID(), VersionNumber: id.Version(), UIName: []byte(`{ "en": "B", "number": 123456789012345678901234567890 }`), Description: []byte("null")},
		{ID: "A", ProjectID: id.ProjectID(), VersionNumber: id.Version(), UIName: []byte(`{ "en": "A" }`)},
	}
	counts := map[string]int64{}
	for i := range v.NumField() {
		counts[v.Type().Field(i).Tag.Get("json")] = int64(v.Field(i).Len())
	}
	descriptor, err := releasedescriptor.Decode(encodeJSON(t, releasedescriptor.Document{Format: 1, ProjectID: id.ProjectID(), Version: id.Version(), Actors: map[string]releasedescriptor.Actor{"owner": {ActorID: "owner", DisplayName: "Original owner"}}, ProjectModules: map[string]string{id.ProjectID(): "example.test/project"}, OntologyModules: map[string]string{}}), id.ProjectID(), id.Version())
	if err != nil {
		t.Fatal(err)
	}
	var evidence projectsnapshot.Evidence
	if draft {
		evidence, err = projectsnapshot.DraftEvidence(id, descriptor, []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	} else {
		sections := map[string]releasecontent.Section{}
		for name, n := range counts {
			sections[name] = releasecontent.Section{Rows: n, SHA256: strings.Repeat("a", 64)}
		}
		receipt, decodeErr := releasecontent.DecodeVerified(encodeJSON(t, receiptFile{Format: 1, Sections: sections}), id.ProjectID(), id.Version())
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		evidence, err = projectsnapshot.VerifiedEvidence(receipt, descriptor, []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	}
	if err != nil {
		t.Fatal(err)
	}
	value, err := projectsnapshot.NewComplete(projectsnapshot.ProjectInput{Content: c, Evidence: evidence, Counts: counts, Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRoundTripAndStableOrdering(t *testing.T) {
	for _, draft := range []bool{false, true} {
		value := fixture(t, draft)
		files, err := Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(files, DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		root, _ := value.Root()
		before, _ := value.Input(root)
		after, _ := decoded.Snapshot().Input(root)
		slices.SortFunc(before.Content.Fields, func(a, b projectsnapshot.FieldRecord) int { return strings.Compare(a.ID, b.ID) })
		slices.SortFunc(after.Content.Fields, func(a, b projectsnapshot.FieldRecord) int { return strings.Compare(a.ID, b.ID) })
		if diff := cmp.Diff(before.Content, after.Content); diff != "" {
			t.Fatal(diff)
		}
		if diff := cmp.Diff(before.Evidence.Descriptors().Document(), after.Evidence.Descriptors().Document()); diff != "" {
			t.Fatal(diff)
		}
		if !draft {
			a, _ := before.Evidence.CaptureSections()
			b, _ := after.Evidence.CaptureSections()
			if diff := cmp.Diff(a, b); diff != "" {
				t.Fatal(diff)
			}
		}
		// Caller changes never mutate the detached original or imported value.
		descriptors := after.Evidence.Descriptors().Document()
		descriptors.Actors["owner"] = releasedescriptor.Actor{}
		before.Content.Fields[0].UIName[0] = '!'
		for path := range files {
			files[path][0] = '!'
		}
		again, err := Encode(decoded.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		original, err := Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(original, again); diff != "" {
			t.Fatal(diff)
		}
		fresh, _ := value.Input(root)
		slices.Reverse(fresh.Content.Fields)
		reordered, err := projectsnapshot.NewComplete(fresh, nil)
		if err != nil {
			t.Fatal(err)
		}
		sorted, err := Encode(reordered)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(original, sorted); diff != "" {
			t.Fatal(diff)
		}
	}
}

func TestRejectMalformedAndIncompleteBundles(t *testing.T) {
	tests := map[string]func(Files){
		"unsupported format": func(f Files) {
			f[ManifestPath] = bytes.Replace(f[ManifestPath], []byte(`"schema_version": 3`), []byte(`"schema_version": 1`), 1)
		},
		"unknown field":   func(f Files) { f[ManifestPath] = append([]byte("{\n  \"unknown\": true,"), f[ManifestPath][1:]...) },
		"duplicate key":   func(f Files) { f[ManifestPath] = append([]byte("{\n  \"schema_version\": 3,"), f[ManifestPath][1:]...) },
		"catalog overlay": func(f Files) { f["releases/index.yaml"] = []byte("[]") },
		"unsafe path":     func(f Files) { f["../escape"] = []byte("[]") },
		"backslash":       func(f Files) { f[`projects\escape`] = []byte("[]") },
		"missing section": func(f Files) {
			editContext(t, f, func(c *contextFile) { delete(c.Sections, "weave_examples_archive") })
		},
		"null section": func(f Files) { editContext(t, f, func(c *contextFile) { c.Sections["weave_examples_archive"] = nil }) },
		"repeated row": func(f Files) {
			editContext(t, f, func(c *contextFile) {
				r := c.Sections["weave_fields_archive"]
				c.Sections["weave_fields_archive"] = append(r, r[0])
			})
		},
		"wrong receipt": func(f Files) {
			editContext(t, f, func(c *contextFile) {
				c.Evidence.Receipt.Sections["weave_examples_archive"] = releasecontent.Section{Rows: 1, SHA256: strings.Repeat("a", 64)}
			})
		},
		"lost dependency inventory": func(f Files) { editContext(t, f, func(c *contextFile) { c.Dependencies = nil }) },
		"missing scalar":            func(f Files) { editRow(t, f, func(row map[string]json.RawMessage) { delete(row, "deprecated") }) },
		"nonnullable null": func(f Files) {
			editRow(t, f, func(row map[string]json.RawMessage) { row["deprecated"] = json.RawMessage(`null`) })
		},
		"unknown scalar": func(f Files) {
			editRow(t, f, func(row map[string]json.RawMessage) { row["future"] = json.RawMessage(`null`) })
		},
		"missing file": func(f Files) {
			for path := range f {
				if strings.Contains(path, "/rows/") {
					delete(f, path)
					return
				}
			}
		},
		"invalid utf8": func(f Files) { f[ManifestPath] = append(f[ManifestPath], 0xff) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := Encode(fixture(t, false))
			if err != nil {
				t.Fatal(err)
			}
			mutate(f)
			if _, err := Decode(f, DefaultLimits()); err == nil {
				t.Fatal("accepted damaged bundle")
			}
		})
	}
}

func editContext(t *testing.T, f Files, edit func(*contextFile)) {
	t.Helper()
	var index manifest
	if err := json.Unmarshal(f[ManifestPath], &index); err != nil {
		t.Fatal(err)
	}
	var c contextFile
	if err := json.Unmarshal(f[index.Projects[0].Path], &c); err != nil {
		t.Fatal(err)
	}
	edit(&c)
	data, err := canonical(c)
	if err != nil {
		t.Fatal(err)
	}
	index.Projects[0] = f.put(index.Projects[0].Path, data)
	f[ManifestPath], err = canonical(index)
	if err != nil {
		t.Fatal(err)
	}
}
func editRow(t *testing.T, f Files, edit func(map[string]json.RawMessage)) {
	t.Helper()
	editContext(t, f, func(c *contextFile) {
		refs := c.Sections["weave_fields_archive"]
		var row map[string]json.RawMessage
		if err := json.Unmarshal(f[refs[0].Path], &row); err != nil {
			t.Fatal(err)
		}
		edit(row)
		data, err := canonical(row)
		if err != nil {
			t.Fatal(err)
		}
		refs[0] = f.put(refs[0].Path, data)
	})
}

func TestBudgetsAndZeroValues(t *testing.T) {
	if _, err := Encode(projectsnapshot.CompleteProject{}); err == nil {
		t.Fatal("accepted empty complete value")
	}
	f, err := Encode(fixture(t, false))
	if err != nil {
		t.Fatal(err)
	}
	for _, limits := range []Limits{{}, {MaxBytes: 1, MaxFiles: 1000, MaxProjects: 1}, {MaxBytes: 1 << 20, MaxFiles: 1, MaxProjects: 1}} {
		if _, err := Decode(f, limits); err == nil {
			t.Fatal("ignored decode budget")
		}
	}
}
