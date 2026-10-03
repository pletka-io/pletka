package conformance

// Exemption records why one read method does not take a scope.
//
// The allowlist is the phase-out plan: visible, shrinking, reviewable in a
// diff. It has permanent members and that is correct -- the rule is that
// every entry names why, not that the list reaches zero.
type Exemption struct {
	// Reason says why this read does not name a version. Written for
	// someone who was not there.
	Reason string
	// Permanent marks a read that is correct unscoped by nature, not one
	// waiting for its slice to convert.
	Permanent bool
	// ConvertsWith names what will remove a temporary entry -- a slice
	// conversion, a ticket, a plan.
	ConvertsWith string
	// CallerObligation is required on a permanent entry. An exemption
	// covers the slice, never its callers; this says what a caller must
	// still do.
	CallerObligation string
}

// Three shapes of temporary entry, so a converter can tell at a glance what
// kind of work each one is.
const (
	// ambientVersioned: already version-aware, but through
	// auth.ProjectVersionFromContext -- the ambient accessor the read model
	// replaces. Behavior is correct today; the mechanism is the deprecated
	// one, so converting is a signature change, not a bug fix.
	ambientVersioned = "Version-aware today, but through auth.ProjectVersionFromContext " +
		"rather than a scope parameter. Behavior is correct; the mechanism is the one " +
		"being retired, so converting this is a signature change rather than a bug fix. "

	// trulyUnscoped: reads live rows whatever scope was asked for.
	trulyUnscoped = "Reads live rows whatever scope was asked for, so a pinned view shows " +
		"current data with no signal. "
)

// Converters: what will remove each temporary entry. Named so the phase-out
// plan is readable as a list of pieces of work rather than a repeated string.
const (
	convAttribution = "the attribution slice conversion"
	convCategory    = "the category slice conversion (spec Part 3, sequencing step 4)"
	convCollection  = "the collection slice conversion (spec Part 3, sequencing step 5)"
	convExample     = "the example slice conversion (release-completeness read side)"
	convField       = "the field slice conversion (spec Part 3, sequencing step 4)"
	convFieldUsage  = "the field usage-counts conversion (spec Part 3, sequencing step 4)"
	convModel       = "the model slice conversion (spec Part 3, sequencing step 5)"
	convVocabulary  = "the vocabulary slice conversion (spec Part 3, sequencing step 4)"
)

// UnscopedReadAllowlist maps "slice.Method" to its exemption.
//
// Populated 2026-10-03 from a scan of the live tree: 53 read-shaped methods
// across 8 versioned slices, of which 6 were already scoped (the override
// conversion) and 47 are listed here.
var UnscopedReadAllowlist = map[string]Exemption{
	// --- attribution -------------------------------------------------
	// Migration 020: "Credit is part of what a release cites." Both of
	// these read the live credit tables, so a pinned view shows today's
	// contributors rather than the release's.
	"attribution.ListForProject": {
		Reason:       trulyUnscoped + "Credit is release content as of migration 020, so a release cites whoever is credited now.",
		ConvertsWith: convAttribution,
	},
	"attribution.ListAllActors": {
		Reason:       trulyUnscoped + "Lists the actors available to credit; under a release it offers people added since.",
		ConvertsWith: convAttribution,
	},

	// --- category ----------------------------------------------------
	"category.List": {
		Reason:       trulyUnscoped + "A pinned view lists categories renamed or added after the release.",
		ConvertsWith: convCategory,
	},
	"category.Get": {
		Reason:       ambientVersioned + "Reads the archived category when a version is pinned.",
		ConvertsWith: convCategory,
	},
	"category.ListWithCounts": {
		Reason:       ambientVersioned + "The counts beside each category come from the same branch.",
		ConvertsWith: convCategory,
	},

	// --- collection --------------------------------------------------
	"collection.List": {
		Reason:       ambientVersioned,
		ConvertsWith: convCollection,
	},
	"collection.Get": {
		Reason:       ambientVersioned,
		ConvertsWith: convCollection,
	},
	"collection.ListAdoptedExplicit": {
		Reason:       ambientVersioned + "Adopted collections resolve at the reader's version.",
		ConvertsWith: convCollection,
	},
	"collection.ListAdoptedByReference": {
		Reason:       trulyUnscoped + "Its sibling ListAdoptedExplicit branches on the version and this one does not, so the two disagree under a release.",
		ConvertsWith: convCollection,
	},
	"collection.BatchCompositionCounts": {
		Reason:       trulyUnscoped + "Composition counts are computed from live membership, so a pinned collection reports how many fields it holds today.",
		ConvertsWith: convCollection,
	},
	"collection.View": {
		Reason: trulyUnscoped + "View is the surface the generators consume, so a pinned SHACL or Turtle export " +
			"carries a release-era entity with draft fields -- internally contradictory rather than merely stale. The spec's exports section calls this out.",
		ConvertsWith: "the collection slice conversion, which the spec sequences before exports",
	},

	// --- example -----------------------------------------------------
	// The example slice has no version awareness at all. Its archives
	// exist (migration 017) and the release write path fills them; nothing
	// reads them back.
	"example.List": {
		Reason:       trulyUnscoped + "weave_examples_archive exists as of migration 017 and nothing outside the release slice reads it.",
		ConvertsWith: convExample,
	},
	"example.Get": {
		Reason:       trulyUnscoped + "Same gap as example.List: the archive is written and never read back.",
		ConvertsWith: convExample,
	},

	// --- field -------------------------------------------------------
	"field.List": {
		Reason:       ambientVersioned,
		ConvertsWith: convField,
	},
	"field.Get": {
		Reason:       ambientVersioned,
		ConvertsWith: convField,
	},
	"field.GetByIdentifier": {
		Reason:       ambientVersioned,
		ConvertsWith: convField,
	},
	"field.ListAdoptedExplicit": {
		Reason:       ambientVersioned,
		ConvertsWith: convField,
	},
	"field.ListAdoptedByReference": {
		Reason:       trulyUnscoped + "Its sibling ListAdoptedExplicit branches on the version and this one does not.",
		ConvertsWith: convField,
	},
	"field.BatchUsageCounts": {
		Reason:       trulyUnscoped + "Usage counts come from live placements, so a pinned view reports how many models use a field today.",
		ConvertsWith: convFieldUsage,
	},
	"field.BatchUsageRefs": {
		Reason:       trulyUnscoped + "The referencing entities resolve live, so a pinned view can list a model that did not exist at the release.",
		ConvertsWith: convFieldUsage,
	},
	"field.Resolved": {
		Reason:       trulyUnscoped + "Resolution merges base and override data; under a release both halves should come from the archive.",
		ConvertsWith: convField,
	},
	"field.GetModels": {
		Reason:       trulyUnscoped + "Lists the models a field sits in, from live placements.",
		ConvertsWith: convField,
	},
	"field.GetCollections": {
		Reason:       trulyUnscoped + "Lists the collections a field sits in, from live placements.",
		ConvertsWith: convField,
	},
	"field.ListBaseFieldCategories": {
		Reason:       trulyUnscoped + "Categories are archived, so a pinned view can show one added after the release.",
		ConvertsWith: convField,
	},
	"field.ListBaseFieldCategoryAssignments": {
		Reason:       trulyUnscoped + "The assignment of a field to a category is release content and reads live here.",
		ConvertsWith: convField,
	},
	"field.FindByPathSequence": {
		Reason:       trulyUnscoped + "Finds a field by its ontology path against live rows; a pinned lookup can match a field whose path changed since.",
		ConvertsWith: convField,
	},

	// --- model -------------------------------------------------------
	"model.List": {
		Reason:       ambientVersioned,
		ConvertsWith: convModel,
	},
	"model.Get": {
		Reason:       ambientVersioned,
		ConvertsWith: convModel,
	},
	"model.ListAdoptedExplicit": {
		Reason:       ambientVersioned,
		ConvertsWith: convModel,
	},
	"model.ListAdoptedByReference": {
		Reason:       trulyUnscoped + "Its sibling ListAdoptedExplicit branches on the version and this one does not.",
		ConvertsWith: convModel,
	},
	"model.BatchCompositionCounts": {
		Reason:       trulyUnscoped + "Composition counts are computed from live membership.",
		ConvertsWith: convModel,
	},
	"model.BatchInUse": {
		Reason:       trulyUnscoped + "Whether a model is in use is answered from live references.",
		ConvertsWith: convModel,
	},
	"model.ListConnectedModelIDs": {
		Reason:       trulyUnscoped + "The connection graph is walked over live rows, so a pinned view can traverse an edge created after the release.",
		ConvertsWith: convModel,
	},
	"model.View": {
		Reason: trulyUnscoped + "View is the surface the generators consume, so a pinned export carries a " +
			"release-era entity with draft fields and overrides. The spec's exports section calls this out as live today.",
		ConvertsWith: "the model slice conversion, which the spec sequences before exports",
	},

	// --- override ----------------------------------------------------
	"override.ListPlacements": {
		Reason: trulyUnscoped + "Reads weave_collection_placements live although its archive exists as of " +
			"migration 019. Named in the phase-1 plan's 'After this plan' section as converting with the slice that renders it.",
		ConvertsWith: "the collection slice conversion (placements have an archive as of Part 1)",
	},

	// --- vocabulary: project content ---------------------------------
	"vocabulary.ListProjectVocabularies": {
		Reason:       trulyUnscoped + "A pinned view lists vocabularies attached after the release.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.ListProjectConceptLists": {
		Reason:       trulyUnscoped + "A pinned view lists concept lists created after the release, and current membership for ones that existed at it.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.GetProjectConceptList": {
		Reason:       ambientVersioned + "The one vocabulary read that already branches; its own comment says so.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.ListBroader": {
		Reason:       trulyUnscoped + "weave_concept_broader is archived, so a pinned term's hierarchy should come from the release.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.ListNarrower": {
		Reason:       trulyUnscoped + "The narrower side of the same archived hierarchy as ListBroader.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.SearchConceptListSourceEntries": {
		Reason:       trulyUnscoped + "Searches within a project's concept list, whose entries are archived.",
		ConvertsWith: convVocabulary,
	},

	// --- vocabulary: not release-scoped content (permanent) ----------
	"vocabulary.ResolveEntry": {
		Reason: "Resolves a URI to a vocabulary entry globally. It takes no project and no list -- " +
			"a URI means the same thing everywhere -- so there is no version for it to read at.",
		Permanent: true,
		CallerObligation: "A caller rendering released content must not assume a resolved label matches " +
			"what the release captured; the archived entry is the release's answer, and this is a current lookup.",
	},
	"vocabulary.ListAdminVocabularies": {
		Reason: "An administrative inventory of every vocabulary in the system, with no project and no " +
			"version. Auditing the working state is its purpose, which is one of the shapes the spec names as correctly permanent.",
		Permanent: true,
		CallerObligation: "It is a system inventory, never a project's released vocabulary list; a project " +
			"view must use ListProjectVocabularies and name its scope.",
	},
	"vocabulary.SearchVocabularyEntries": {
		Reason: "A term picker: it searches a vocabulary's entries so a curator can choose one. Choosing " +
			"is an editing action and editing happens on the draft, so searching current entries is the correct behavior.",
		Permanent: true,
		CallerObligation: "An editor-only surface. A read-only release view must not source displayed values " +
			"from a picker -- it reads what the release captured.",
	},
	"vocabulary.SearchVocabularyEntriesWithParent": {
		Reason: "The parent-scoped form of the same picker, used when a concept list is narrowed to a " +
			"parent term. Editing-time search over current entries.",
		Permanent:        true,
		CallerObligation: "An editor-only surface, as with SearchVocabularyEntries.",
	},
	"vocabulary.SearchVocabularyEntriesDegradable": {
		Reason: "The picker again, in the form that reports a degraded vocabulary service instead of " +
			"failing. Same editing-time purpose, different error contract.",
		Permanent:        true,
		CallerObligation: "An editor-only surface, as with SearchVocabularyEntries.",
	},
	"vocabulary.SearchConceptListEntries": {
		Reason: "Searches the entries already in a concept list, as a picker while editing that list. " +
			"It takes no project id and answers an editing question.",
		Permanent: true,
		CallerObligation: "An editor-only surface. Rendering a released list's membership is " +
			"GetProjectConceptList's job, and that one names its scope.",
	},
}
