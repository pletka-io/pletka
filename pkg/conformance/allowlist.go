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
	// Added when the scanner inverted to a mutation denylist and three
	// more versioned slices were registered.
	convNamespace   = "the namespacebinding slice conversion"
	convProject     = "the project slice conversion"
	convOntologyVer = "the projectontologyversion slice conversion"
)

// UnscopedReadAllowlist maps "slice.Method" to its exemption.
//
// Populated 2026-10-03 from a scan of the live tree: 53 read-shaped methods
// across 8 versioned slices, of which 6 were already scoped (the override
// conversion) and 47 are listed here.
var UnscopedReadAllowlist = map[string]Exemption{
	// --- surfaced when the scanner inverted to a mutation denylist -----
	"category.Stats": {
		Reason:       trulyUnscoped + "Derived counts over live rows, so a pinned view reports today's totals.",
		ConvertsWith: convCategory,
	},
	"category.IsInUse": {
		Reason:       trulyUnscoped + "Answers whether a category is used, from live placements.",
		ConvertsWith: convCategory,
	},
	"field.Stats": {
		Reason:       trulyUnscoped + "Derived counts over live rows.",
		ConvertsWith: convField,
	},
	"field.IsInUse": {
		Reason:       trulyUnscoped + "Answers whether a field is used, from live placements.",
		ConvertsWith: convField,
	},
	"collection.Stats": {
		Reason:       trulyUnscoped + "Derived counts over live rows.",
		ConvertsWith: convCollection,
	},
	"collection.LookupView": {
		Reason:       trulyUnscoped + "The same resolved-field surface as collection.View under a different name, so it carries the same release-era-entity-with-draft-fields problem.",
		ConvertsWith: convCollection,
	},
	"collection.LookupByID": {
		Reason:       trulyUnscoped + "A live lookup by id; a pinned view can resolve a collection created after the release.",
		ConvertsWith: convCollection,
	},
	"collection.AnchorIndex": {
		Reason:       trulyUnscoped + "Builds the anchor index from live membership.",
		ConvertsWith: convCollection,
	},
	"collection.CategoriesUsed": {
		Reason:       trulyUnscoped + "Lists the categories a collection uses, from live placements.",
		ConvertsWith: convCollection,
	},
	"collection.ScopeClasses": {
		Reason:       trulyUnscoped + "Derives scope classes from live field paths.",
		ConvertsWith: convCollection,
	},
	"collection.ForkOriginsForProject": {
		Reason:       trulyUnscoped + "Fork origins resolve against live source entities.",
		ConvertsWith: convCollection,
	},
	"model.Stats": {
		Reason:       trulyUnscoped + "Derived counts over live rows.",
		ConvertsWith: convModel,
	},
	"model.ScopeClasses": {
		Reason:       trulyUnscoped + "Derives scope classes from live field paths.",
		ConvertsWith: convModel,
	},
	"model.ForkOriginsForProject": {
		Reason:       trulyUnscoped + "Fork origins resolve against live source entities.",
		ConvertsWith: convModel,
	},
	"example.TargetName": {
		Reason: trulyUnscoped + "Resolves an example's TARGET entity name -- a model or collection -- " +
			"so a pinned example can show a name changed since the release. It reads no example rows, " +
			"which is why the example slice's own conversion left it behind: it converts when the slices " +
			"that own those names do.",
		ConvertsWith: convModel + " and " + convCollection,
	},
	"override.EntityFingerprint": {
		Reason:       trulyUnscoped + "Fingerprints live placement rows to detect concurrent edits, which is a draft-time question.",
		ConvertsWith: convCollection,
	},
	"vocabulary.VocabularyRoots": {
		Reason:       trulyUnscoped + "Browses a vocabulary's curated roots from live entries.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.VocabularyChildren": {
		Reason:       trulyUnscoped + "Walks a vocabulary's hierarchy from live entries.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.VocabularyProjectID": {
		Reason:       trulyUnscoped + "Resolves which project owns a vocabulary, from the live row.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.ConceptListProjectID": {
		Reason:       trulyUnscoped + "Resolves which project owns a concept list, from the live row.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.ConceptListMemberURIs": {
		Reason:       trulyUnscoped + "Lists a concept list's member URIs live; under a release these should be the archived members.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.ConceptURIInLists": {
		Reason:       trulyUnscoped + "Answers which lists contain a URI, over live membership.",
		ConvertsWith: convVocabulary,
	},
	"vocabulary.RenderConceptListSKOS": {
		Reason:       trulyUnscoped + "An export renderer: it serializes a concept list to SKOS from live rows, so a pinned export does not match the release it claims. The spec's exports section is explicit that an export leaves the system.",
		ConvertsWith: convVocabulary,
	},
	"namespacebinding.Get": {
		Reason:       trulyUnscoped + "Reads a live binding.",
		ConvertsWith: convNamespace,
	},
	"namespacebinding.List": {
		Reason:       trulyUnscoped + "Lists live bindings.",
		ConvertsWith: convNamespace,
	},
	"namespacebinding.ListForProject": {
		Reason:       trulyUnscoped + "Lists a project's live bindings; prefixes are archived, so a pinned view can show one added since.",
		ConvertsWith: convNamespace,
	},
	"namespacebinding.GetGlobal": {
		Reason:       trulyUnscoped + "Reads a global binding, which is not project content.",
		ConvertsWith: convNamespace,
	},
	"namespacebinding.ListGlobal": {
		Reason:       trulyUnscoped + "Lists global bindings, which are not project content.",
		ConvertsWith: convNamespace,
	},
	"namespacebinding.ListGlobalBrowse": {
		Reason:       trulyUnscoped + "The browse form of the global list.",
		ConvertsWith: convNamespace,
	},
	"namespacebinding.StatsGlobal": {
		Reason:       trulyUnscoped + "Counts over global bindings.",
		ConvertsWith: convNamespace,
	},
	"project.Get": {
		Reason:       trulyUnscoped + "Reads the live project row; weave_projects is archived.",
		ConvertsWith: convProject,
	},
	"project.GetByID": {
		Reason:       trulyUnscoped + "Reads the live project row by id.",
		ConvertsWith: convProject,
	},
	"project.List": {
		Reason:       trulyUnscoped + "Lists live projects.",
		ConvertsWith: convProject,
	},
	"project.ListVisible": {
		Reason:       trulyUnscoped + "Lists projects visible to the caller, from live rows.",
		ConvertsWith: convProject,
	},
	"project.ListChildren": {
		Reason:       trulyUnscoped + "Lists children from live inheritance rows; weave_project_inheritance is archived.",
		ConvertsWith: convProject,
	},
	"project.ListOwnerInstitutions": {
		Reason:       trulyUnscoped + "Lists owning institutions from live rows.",
		ConvertsWith: convProject,
	},
	"project.ListVisibleOwnerInstitutions": {
		Reason:       trulyUnscoped + "The visibility-filtered form of the same live read.",
		ConvertsWith: convProject,
	},
	"project.OwnersForProjects": {
		Reason:       trulyUnscoped + "Batch owner lookup over live rows.",
		ConvertsWith: convProject,
	},
	"project.StatsForProjects": {
		Reason:       trulyUnscoped + "Batch counts over live rows.",
		ConvertsWith: convProject,
	},
	"project.ResolvedOntologyVersions": {
		Reason:       trulyUnscoped + "Resolves which ontology versions a project uses from live links; those links are archived.",
		ConvertsWith: convProject,
	},
	"projectontologyversion.Get": {
		Reason:       trulyUnscoped + "Reads a live ontology-version link.",
		ConvertsWith: convOntologyVer,
	},
	"projectontologyversion.ListView": {
		Reason:       trulyUnscoped + "Lists a project's ontology versions live.",
		ConvertsWith: convOntologyVer,
	},
	"projectontologyversion.PaneView": {
		Reason:       trulyUnscoped + "Builds the settings pane from live links.",
		ConvertsWith: convOntologyVer,
	},
	"projectontologyversion.Stats": {
		Reason:       trulyUnscoped + "Counts over live links.",
		ConvertsWith: convOntologyVer,
	},
	"projectontologyversion.ListAvailableVersions": {
		Reason:       trulyUnscoped + "Lists what a project could pin to: a catalog question, not release content.",
		ConvertsWith: convOntologyVer,
	},
	"projectontologyversion.ListAvailableExtensions": {
		Reason:       trulyUnscoped + "The same catalog question for extensions.",
		ConvertsWith: convOntologyVer,
	},
	"projectontologyversion.ListBaseOntologies": {
		Reason:       trulyUnscoped + "The same catalog question for base ontologies.",
		ConvertsWith: convOntologyVer,
	},
	"project.CanEdit": {
		Reason:           "A permission check, not a content read. The spec's first decision says non-versioned data is a declared boundary: authorization is answered about the person and the project now, at any scope.",
		Permanent:        true,
		CallerObligation: "A caller must not infer from an edit permission that it may render draft content under a release.",
	},
	"project.CanRead": {
		Reason:           "A permission check, as with CanEdit: answered about the caller now, not at a version.",
		Permanent:        true,
		CallerObligation: "Read permission says who may see the project, never which version they get; the caller still names the scope.",
	},
	"project.CheckIDPrefix": {
		Reason:           "Validates that an id prefix is free before a project is created, against the live namespace where the new project will live.",
		Permanent:        true,
		CallerObligation: "A creation-time check; nothing rendering a release should call it.",
	},
	"collection.ForkFromSource": {
		Reason:           "A mutation: it creates a forked collection. Mutations run hot by design and take no scope, which is structural in the Store/Reader split rather than a rule to remember.",
		Permanent:        true,
		CallerObligation: "A write path reads the draft deliberately; a caller under a release must not invoke it to materialize release content.",
	},
	"model.ForkFromSource": {
		Reason:           "A mutation: it creates a forked model. Mutations run hot by design and take no scope, which is structural in the Store/Reader split rather than a rule to remember.",
		Permanent:        true,
		CallerObligation: "A write path reads the draft deliberately; a caller under a release must not invoke it to materialize release content.",
	},

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
