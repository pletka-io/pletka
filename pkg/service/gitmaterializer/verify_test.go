package gitmaterializer

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// verifyFixture writes a minimal valid tree: one category, two fields (one
// literal-valued, one referencing a class), one model with two placements,
// one concept list, no vendor. Returns the root.
func verifyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"project.yaml": `schema_version: 1
project:
  id: TPC
  system_name: test_project
  title: {en: Test}
namespaces:
  effective_bindings:
    - {prefix: base, namespace: https://example.org/base/, source: ontology}
ontologies:
  linked_versions:
    - {prefix: base, version: "1.0", is_primary: true}
manifests:
  adoptions: adoptions/index.yaml
`,
		"pletka.mod":                            "schema_version: 1\nmodule:\n  path: pletka.io/orgs/test/projects/TPC\n  project_id: TPC\nrequire:\n  ontologies:\n    - {module: ontology.pletka.io/base, version: \"1.0\"}\n  projects: []\nreplace: []\n",
		"adoptions/index.yaml":                  "schema_version: 1\nreceipts: []\n",
		"categories/TPC.CAT.1.yaml":             "semantic_id: TPC.CAT.1\nsystem_name: general\nui_name: {en: General}\nstatus: draft\n",
		"concept-lists/TPC.CL.1.yaml":           "semantic_id: TPC.CL.1\nsystem_name: kinds\nui_name: {en: Kinds}\nstatus: draft\n",
		"fields/TPCF.1/field.yaml":              "semantic_id: TPCF.1\nsystem_name: name\nui_name: {en: Name}\nontology_scope: base:Thing\npath_elements: [base:name, xsd:string]\nexpected_value_type: String\nstatus: draft\n",
		"fields/TPCF.1/base-override.yaml":      "field_id: TPCF.1\nentity_type: \"\"\ncategory_id: TPC.CAT.1\n",
		"fields/TPCF.2/field.yaml":              "semantic_id: TPCF.2\nsystem_name: knows\nui_name: {en: Knows}\nontology_scope: base:Person\npath_elements: [base:knows, base:Person]\nexpected_value_type: Model\nstatus: draft\n",
		"fields/TPCF.2/base-override.yaml":      "field_id: TPCF.2\nentity_type: \"\"\ncategory_id: TPC.CAT.1\n",
		"models/TPCM.1/model.yaml":              "semantic_id: TPCM.1\nsystem_name: person\nui_name: {en: Person}\nontology_scope: base:Person\nstatus: draft\n",
		"models/TPCM.1/overrides/TPCF.1@a.yaml": "field_id: TPCF.1\nentity_type: model\nentity_id: TPCM.1\nposition: 0\ncategory_id: TPC.CAT.1\n",
		"models/TPCM.1/overrides/TPCF.2@b.yaml": "field_id: TPCF.2\nentity_type: model\nentity_id: TPCM.1\nposition: 1\ncategory_id: TPC.CAT.1\nrefs:\n  - {ref_type: concept_list, semantic_id: TPC.CL.1, position: 0}\n",
	}
	for rel, body := range files {
		write(t, root, rel, body)
	}
	return root
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func codes(rep *VerifyReport) []string {
	out := make([]string, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		out = append(out, f.Code)
	}
	sort.Strings(out)
	return out
}

func TestVerify_CleanTreeHasNoStructureFindings(t *testing.T) {
	rep, err := Verify(context.Background(), verifyFixture(t), VerifyOptions{SkipOntology: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("findings: %v", rep.Findings)
	}
}

func TestVerify_StructureFindings(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(root string)
		want   []string // sorted codes
		file   string   // file of the first finding
		line   int
	}{
		{"unknown key", func(r string) {
			write(t, r, "fields/TPCF.1/field.yaml", "semantic_id: TPCF.1\nsystem_name: name\nui_name: {en: Name}\nontology_scope: base:Thing\npath_element: [base:name]\nstatus: draft\n")
		}, []string{"unknown-key"}, "fields/TPCF.1/field.yaml", 5},
		{"id mismatch", func(r string) {
			write(t, r, "fields/TPCF.2/field.yaml", "semantic_id: TPCF.9\nsystem_name: knows\nui_name: {en: K}\nontology_scope: base:Person\npath_elements: [base:knows, base:Person]\nstatus: draft\n")
		}, []string{"semantic-id"}, "fields/TPCF.2/field.yaml", 1},
		{"foreign prefix", func(r string) {
			write(t, r, "categories/TPC.CAT.1.yaml", "semantic_id: OTHER.CAT.1\nsystem_name: general\nui_name: {en: G}\n")
		}, []string{"semantic-id"}, "categories/TPC.CAT.1.yaml", 1},
		{"unknown file", func(r string) { write(t, r, "fields/TPCF.1/notes.txt", "x") }, []string{"unknown-file"}, "fields/TPCF.1/notes.txt", 0},
		{"bad yaml", func(r string) { write(t, r, "models/TPCM.1/model.yaml", "semantic_id: [unclosed\n") }, []string{"decode"}, "models/TPCM.1/model.yaml", 0},
		{"unresolved field", func(r string) {
			write(t, r, "models/TPCM.1/overrides/TPCF.7@c.yaml", "field_id: TPCF.7\nentity_type: model\nentity_id: TPCM.1\n")
		}, []string{"unresolved-field"}, "models/TPCM.1/overrides/TPCF.7@c.yaml", 1},
		{"unresolved category", func(r string) {
			write(t, r, "fields/TPCF.1/base-override.yaml", "field_id: TPCF.1\nentity_type: \"\"\ncategory_id: TPC.CAT.9\n")
		}, []string{"unresolved-category"}, "fields/TPCF.1/base-override.yaml", 3},
		{"unresolved concept list", func(r string) {
			write(t, r, "models/TPCM.1/overrides/TPCF.2@b.yaml", "field_id: TPCF.2\nentity_type: model\nentity_id: TPCM.1\nrefs:\n  - {ref_type: concept_list, semantic_id: TPC.CL.9}\n")
		}, []string{"unresolved-concept-list"}, "models/TPCM.1/overrides/TPCF.2@b.yaml", 4},
		{"unresolved collection", func(r string) {
			write(t, r, "models/TPCM.1/overrides/TPCF.1@a.yaml", "field_id: TPCF.1\nentity_type: model\nentity_id: TPCM.1\npart_of_collection_id: TPCC.3\n")
		}, []string{"unresolved-collection"}, "models/TPCM.1/overrides/TPCF.1@a.yaml", 4},
		{"orphan overrides", func(r string) {
			write(t, r, "models/TPCM.2/overrides/TPCF.1@d.yaml", "field_id: TPCF.1\nentity_type: model\nentity_id: TPCM.2\n")
		}, []string{"orphan-overrides"}, "models/TPCM.2/overrides/TPCF.1@d.yaml", 0},
		{"missing sum", func(r string) {
			write(t, r, "vendor/ontologies/ontology.pletka.io/base/1.0/ontology.yaml", "schema_version: 1\nontology: {module: ontology.pletka.io/base, version: \"1.0\", prefixes: [base], namespace: https://example.org/base/}\nsources: {files: [], primary: \"\"}\n")
		}, []string{"vendor-checksum"}, "pletka.sum", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := verifyFixture(t)
			tc.mutate(root)
			rep, err := Verify(context.Background(), root, VerifyOptions{SkipOntology: true})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, codes(rep)); diff != "" {
				t.Fatalf("codes (-want +got):\n%s\n%v", diff, rep.Findings)
			}
			if rep.Findings[0].File != tc.file || rep.Findings[0].Line != tc.line {
				t.Fatalf("location %s:%d, want %s:%d", rep.Findings[0].File, rep.Findings[0].Line, tc.file, tc.line)
			}
		})
	}
}

// childFixture is a child whose placement uses a parent field via an
// adoption receipt. Returns the child root and the parent root.
func childFixture(t *testing.T) (child, parent string) {
	t.Helper()
	parent = verifyFixture(t)
	child = t.TempDir()
	write(t, child, "project.yaml", "schema_version: 1\nproject: {id: TCH, system_name: child, title: {en: Child}}\ninheritance:\n  parents:\n    - {project_id: TPC, is_primary: true, canonical_order: 0, source_mode: draft}\nnamespaces:\n  effective_bindings:\n    - {prefix: base, namespace: https://example.org/base/, source: ontology}\nontologies:\n  linked_versions:\n    - {prefix: base, version: \"1.0\", is_primary: true}\nmanifests: {adoptions: adoptions/index.yaml}\n")
	write(t, child, "pletka.mod", "schema_version: 1\nmodule: {path: pletka.io/orgs/test/projects/TCH, project_id: TCH}\nrequire:\n  ontologies:\n    - {module: ontology.pletka.io/base, version: \"1.0\"}\n  projects:\n    - {module: pletka.io/orgs/test/projects/TPC, project_id: TPC, mode: parent, version: draft}\nreplace: []\n")
	write(t, child, "adoptions/index.yaml", "schema_version: 1\nreceipts:\n  - {entity_type: field, entity_id: TPCF.1, file: field-TPCF.1.yaml}\n")
	write(t, child, "adoptions/field-TPCF.1.yaml", "schema_version: 1\nadoption:\n  entity_type: field\n  entity_id: TPCF.1\n  source: {project_id: TPC, entity_id: TPCF.1}\n  contexts:\n    - {context_entity_type: model, context_entity_id: TCHM.1}\n")
	write(t, child, "categories/TCH.CAT.1.yaml", "semantic_id: TCH.CAT.1\nsystem_name: own\nui_name: {en: Own}\n")
	write(t, child, "models/TCHM.1/model.yaml", "semantic_id: TCHM.1\nsystem_name: thing\nui_name: {en: Thing}\nontology_scope: base:Thing\nstatus: draft\n")
	write(t, child, "models/TCHM.1/overrides/TPCF.1@a.yaml", "field_id: TPCF.1\nentity_type: model\nentity_id: TCHM.1\ncategory_id: TCH.CAT.1\n")
	return child, parent
}

func TestVerify_ParentViaFlag(t *testing.T) {
	child, parent := childFixture(t)
	rep, err := Verify(context.Background(), child, VerifyOptions{ParentDirs: []string{parent}, SkipOntology: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("findings: %v", rep.Findings)
	}
}

func TestVerify_ParentAbsentIsOneFinding(t *testing.T) {
	child, _ := childFixture(t)
	rep, err := Verify(context.Background(), child, VerifyOptions{SkipOntology: true})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"parent-tree-missing"}, codes(rep)); diff != "" {
		t.Fatalf("(-want +got):\n%s\n%v", diff, rep.Findings)
	}
}

func TestVerify_AdoptionSourceMissingInParent(t *testing.T) {
	child, parent := childFixture(t)
	write(t, child, "adoptions/field-TPCF.1.yaml", "schema_version: 1\nadoption:\n  entity_type: field\n  entity_id: TPCF.1\n  source: {project_id: TPC, entity_id: TPCF.42}\n  contexts: []\n")
	rep, err := Verify(context.Background(), child, VerifyOptions{ParentDirs: []string{parent}, SkipOntology: true})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"adoption-missing-source"}, codes(rep)); diff != "" {
		t.Fatalf("(-want +got):\n%s\n%v", diff, rep.Findings)
	}
}

func TestVerify_ParentNotDeclared(t *testing.T) {
	root := verifyFixture(t)
	other := verifyFixture(t) // project TPC again, which TPC does not declare as a parent
	rep, err := Verify(context.Background(), root, VerifyOptions{ParentDirs: []string{other}, SkipOntology: true})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"parent-not-declared"}, codes(rep)); diff != "" {
		t.Fatalf("(-want +got):\n%s\n%v", diff, rep.Findings)
	}
}

const verifyTestRDF = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:base="https://example.org/base/">
  <owl:Ontology rdf:about="https://example.org/base/"/>
  <owl:Class rdf:about="https://example.org/base/Thing"><rdfs:label xml:lang="en">Thing</rdfs:label></owl:Class>
  <owl:Class rdf:about="https://example.org/base/Person"><rdfs:subClassOf rdf:resource="https://example.org/base/Thing"/></owl:Class>
  <owl:DatatypeProperty rdf:about="https://example.org/base/name">
    <rdfs:domain rdf:resource="https://example.org/base/Thing"/>
    <rdfs:range rdf:resource="http://www.w3.org/2001/XMLSchema#string"/>
  </owl:DatatypeProperty>
  <owl:ObjectProperty rdf:about="https://example.org/base/knows">
    <rdfs:domain rdf:resource="https://example.org/base/Person"/>
    <rdfs:range rdf:resource="https://example.org/base/Person"/>
  </owl:ObjectProperty>
</rdf:RDF>`

// vendorBase writes the base ontology under <root>/vendor/ontologies with a
// valid pletka.sum.
func vendorBase(t *testing.T, root string) {
	t.Helper()
	write(t, root, "vendor/ontologies/ontology.pletka.io/base/1.0/ontology.yaml", "schema_version: 1\nontology:\n  module: ontology.pletka.io/base\n  ontology_id: ont-base\n  version_id: ver-base-1\n  version: \"1.0\"\n  slug: base\n  title: Base\n  kind: base\n  namespace: https://example.org/base/\n  prefixes: [base]\nsources:\n  files: [src/base.rdf]\n  primary: src/base.rdf\n")
	write(t, root, "vendor/ontologies/ontology.pletka.io/base/1.0/src/base.rdf", verifyTestRDF)
	h, err := hashDirectoryTree(filepath.Join(root, "vendor", "ontologies", "ontology.pletka.io", "base", "1.0"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "pletka.sum", "ontology.pletka.io/base 1.0 tree:sha256:"+h+"\n")
}

func TestVerify_OntologyCleanTree(t *testing.T) {
	root := verifyFixture(t)
	vendorBase(t, root)
	rep, err := Verify(context.Background(), root, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 || rep.Fields != 2 || rep.Steps != 6 {
		t.Fatalf("findings=%v fields=%d steps=%d", rep.Findings, rep.Fields, rep.Steps)
	}
}

func TestVerify_OntologyFindings(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(root string)
		want   []string
	}{
		{"not offered", func(r string) {
			write(t, r, "fields/TPCF.2/field.yaml", "semantic_id: TPCF.2\nsystem_name: knows\nui_name: {en: K}\nontology_scope: base:Thing\npath_elements: [base:knows, base:Person]\nexpected_value_type: Model\nstatus: draft\n")
		}, []string{"not-offered"}},
		{"path incomplete", func(r string) {
			write(t, r, "fields/TPCF.2/field.yaml", "semantic_id: TPCF.2\nsystem_name: knows\nui_name: {en: K}\nontology_scope: base:Person\npath_elements: [base:knows]\nexpected_value_type: Model\nstatus: draft\n")
		}, []string{"path-incomplete"}},
		{"terminal mismatch", func(r string) {
			write(t, r, "fields/TPCF.1/field.yaml", "semantic_id: TPCF.1\nsystem_name: name\nui_name: {en: N}\nontology_scope: base:Thing\npath_elements: [base:name, xsd:string]\nexpected_value_type: Model\nstatus: draft\n")
		}, []string{"terminal-mismatch"}},
		{"unbound prefix", func(r string) {
			write(t, r, "fields/TPCF.1/field.yaml", "semantic_id: TPCF.1\nsystem_name: name\nui_name: {en: N}\nontology_scope: base:Thing\npath_elements: [zzz:name, xsd:string]\nexpected_value_type: String\nstatus: draft\n")
		}, []string{"unbound-prefix"}},
		{"missing vendored ontology", func(r string) {
			if err := os.RemoveAll(filepath.Join(r, "vendor")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(r, "pletka.sum")); err != nil {
				t.Fatal(err)
			}
		}, []string{"ontology-missing", "ontology-skipped"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := verifyFixture(t)
			vendorBase(t, root)
			tc.mutate(root)
			rep, err := Verify(context.Background(), root, VerifyOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, codes(rep)); diff != "" {
				t.Fatalf("(-want +got):\n%s\n%v", diff, rep.Findings)
			}
		})
	}
}

func TestVerify_OntologiesDirFlag(t *testing.T) {
	root := verifyFixture(t)
	ont := t.TempDir()
	vendorBase(t, ont)
	rep, err := Verify(context.Background(), root, VerifyOptions{OntologiesDir: filepath.Join(ont, "vendor", "ontologies")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 || rep.Steps != 6 {
		t.Fatalf("findings=%v steps=%d", rep.Findings, rep.Steps)
	}
}

// TestVerify_OntologyAliasAndForeignPrefix pins two things the database
// import tolerates and the offline loader must too: an RDF file written
// against a namespace the manifest lists only as an alias, and a file whose
// own xmlns prefix (geo) is bound to another namespace in the tree's
// effective bindings.
func TestVerify_OntologyAliasAndForeignPrefix(t *testing.T) {
	root := verifyFixture(t)
	write(t, root, "project.yaml", `schema_version: 1
project:
  id: TPC
  system_name: test_project
  title: {en: Test}
namespaces:
  effective_bindings:
    - {prefix: base, namespace: https://example.org/base/, source: ontology}
    - {prefix: geo, namespace: http://www.opengis.net/ont/geosparql#, source: global}
ontologies:
  linked_versions:
    - {prefix: base, version: "1.0", is_primary: true}
manifests:
  adoptions: adoptions/index.yaml
`)
	aliasRDF := strings.ReplaceAll(verifyTestRDF, "https://example.org/base/", "https://alias.example.org/base/")
	aliasRDF = strings.Replace(aliasRDF, `xmlns:base="https://alias.example.org/base/"`, `xmlns:geo="https://alias.example.org/base/"`, 1)
	write(t, root, "vendor/ontologies/ontology.pletka.io/base/1.0/ontology.yaml", "schema_version: 1\nontology:\n  module: ontology.pletka.io/base\n  ontology_id: ont-base\n  version_id: ver-base-1\n  version: \"1.0\"\n  slug: base\n  title: Base\n  kind: base\n  namespace: https://example.org/base/\n  namespace_aliases: [https://alias.example.org/base/]\n  prefixes: [base]\nsources:\n  files: [src/base.rdf]\n  primary: src/base.rdf\n")
	write(t, root, "vendor/ontologies/ontology.pletka.io/base/1.0/src/base.rdf", aliasRDF)
	h, err := hashDirectoryTree(filepath.Join(root, "vendor", "ontologies", "ontology.pletka.io", "base", "1.0"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "pletka.sum", "ontology.pletka.io/base 1.0 tree:sha256:"+h+"\n")
	rep, err := Verify(context.Background(), root, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 || rep.Steps != 6 {
		t.Fatalf("findings=%v steps=%d", rep.Findings, rep.Steps)
	}
}
