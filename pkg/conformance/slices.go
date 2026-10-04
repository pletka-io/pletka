package conformance

// VersionedSlice is a slice that owns at least one table with an _archive
// counterpart. The spec's scope rule makes that ownership the definition of
// "versioned": such a slice must read through a scope-bound reader, because
// a release can be asked for its rows.
type VersionedSlice struct {
	Name string // the slice's short name, used in failure messages
	Pkg  string // repo-relative package path holding service.go
	// ArchiveTables are the _archive tables this slice owns. Listed so the
	// registry can be checked against the migrations rather than trusted.
	ArchiveTables []string
}

// ArchiveTablesWithoutASlice records archive tables that belong to no slice,
// with the reason. Without this the claim check could only be satisfied by
// inventing an owner, which would make the registry a fiction again.
//
// An entry here is not an exemption from versioning -- the data is still
// archived and still read somewhere. It says only that the reads do not live
// behind a slice's *Service, so this package's scanner cannot see them.
var ArchiveTablesWithoutASlice = map[string]string{
	"weave_adoptions_archive": "Adoptions are stored at the pkg/weave package level " +
		"(adoption_store.go), not in a slice, and are read through the WeaveStore aggregate. " +
		"Note pkg/weave/adoption_origin.go resolves a source entity with no version while " +
		"reading the adoption row at the reader's version -- a known unscoped read this " +
		"scanner cannot reach because it is not a *Service method.",
	"weave_entity_forks_archive": "Forks are stored at the pkg/weave package level " +
		"(fork_store.go), not in a slice. Same reach limitation as adoptions.",
	"weave_change_log_archive": "The change log is infrastructure: it records what changed " +
		"rather than holding project content a reader asks for at a version.",
	"weave_change_set_archive": "Change sets drive the materializer queue. Same shape as the " +
		"change log -- infrastructure, not content a release is read for.",
}

// VersionedSlices is the registry. Adding a slice here opts it into the
// ratchet; omitting one whose archive table exists fails
// TestEveryArchiveTableIsClaimedByASlice.
var VersionedSlices = []VersionedSlice{
	{
		Name: "attribution",
		Pkg:  "pkg/weave/attribution",
		// Migration 020: "Credit is part of what a release cites." Both
		// archives deviate from the (id, version_number) pattern because
		// neither live table has an id; the archive key is the live
		// primary key plus version_number.
		ArchiveTables: []string{
			"weave_project_attributions_archive",
			"weave_project_actors_archive",
		},
	},
	{Name: "category", Pkg: "pkg/weave/category", ArchiveTables: []string{"weave_categories_archive"}},
	{Name: "collection", Pkg: "pkg/weave/collection", ArchiveTables: []string{"weave_collections_archive"}},
	{Name: "example", Pkg: "pkg/weave/example", ArchiveTables: []string{"weave_examples_archive", "weave_example_values_archive"}},
	{Name: "field", Pkg: "pkg/weave/field", ArchiveTables: []string{"weave_fields_archive"}},
	{Name: "model", Pkg: "pkg/weave/model", ArchiveTables: []string{"weave_models_archive"}},
	{
		Name:          "namespacebinding",
		Pkg:           "pkg/weave/namespacebinding",
		ArchiveTables: []string{"weave_namespace_bindings_archive"},
	},
	{
		Name: "project",
		Pkg:  "pkg/weave/project",
		ArchiveTables: []string{
			"weave_projects_archive",
			"weave_project_inheritance_archive",
		},
	},
	{
		Name:          "projectontologyversion",
		Pkg:           "pkg/weave/projectontologyversion",
		ArchiveTables: []string{"weave_project_ontology_versions_archive"},
	},
	{Name: "override",
		Pkg: "pkg/weave/override",
		ArchiveTables: []string{
			"weave_field_overrides_archive",
			"weave_override_refs_archive",
			"weave_collection_placements_archive",
		},
	},
	{
		Name: "vocabulary",
		Pkg:  "pkg/weave/vocabulary",
		ArchiveTables: []string{
			"weave_vocabularies_archive",
			"weave_vocabulary_entries_archive",
			"weave_concept_lists_archive",
			"weave_concept_list_entries_archive",
			"weave_concept_broader_archive",
		},
	},
}
