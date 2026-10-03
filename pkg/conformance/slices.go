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
		Name: "override",
		Pkg:  "pkg/weave/override",
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
