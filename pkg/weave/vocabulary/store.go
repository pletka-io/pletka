package vocabulary

import (
	"context"
	"io"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Reader is the scope-bound read surface. Every method answers from one scope
// — the draft or one release — fixed when the Reader was obtained. A Reader
// cannot be asked to read "whichever version the request carried", which is
// the ambiguity the Store/Reader split exists to remove.
//
// A release scope answers from the _archive tables and does NOT fall back to
// live. A release that archived nothing answers "not here", because draft
// content served under a release tag is the defect, not a convenience.
//
// The methods return this slice's own view types rather than row types: a
// view is assembled from more than one query (a concept list's own row plus
// the vocabulary decoration beside it), and assembling it is data access, not
// a decision. Keeping that here is what keeps sqlcgen out of the service.
type Reader interface {
	// ListProjectVocabularies lists the vocabularies attached to the project.
	// Under a release this is what that release recorded, so a vocabulary
	// attached afterwards is absent.
	ListProjectVocabularies(ctx context.Context, projectID string) ([]VocabularyView, error)

	// ListProjectConceptLists lists the project's concept lists with their
	// vocabulary decoration. Membership counts come from the same scope as
	// the lists: current membership beside an archived list would describe a
	// list the reader is not looking at.
	ListProjectConceptLists(ctx context.Context, projectID string) ([]ConceptListView, error)

	// GetProjectConceptList returns one list, or (nil, nil) when this scope
	// does not carry it.
	GetProjectConceptList(ctx context.Context, projectID, id string) (*ConceptListView, error)

	// ConceptListMemberURIs returns the URIs held by the given lists.
	ConceptListMemberURIs(ctx context.Context, projectID string, listIDs []string) ([]string, error)

	// ConceptURIInLists reports whether the URI is a member of any of them.
	ConceptURIInLists(ctx context.Context, uri string, listIDs []string) (bool, error)

	// ConceptListProjectID and VocabularyProjectID answer ownership questions.
	// They are reads of the same rows, so they take the scope with everything
	// else rather than quietly reading live.
	ConceptListProjectID(ctx context.Context, listID string) (string, error)
	VocabularyProjectID(ctx context.Context, vocabularyID string) (string, bool, error)

	// VocabularyRoots and VocabularyChildren walk a vocabulary's hierarchy.
	// The bool reports whether the vocabulary is known, distinct from an empty
	// level, so a caller can tell "no such vocabulary" from "no children".
	VocabularyRoots(ctx context.Context, vocabularyID, lang string) ([]VocabularyEntryView, bool, error)
	VocabularyChildren(ctx context.Context, vocabularyID, conceptID, lang string, limit, offset int) ([]VocabularyEntryView, bool, error)

	// ListBroader and ListNarrower read weave_concept_broader, which is
	// archived, so a pinned term's hierarchy comes from the release rather
	// than from edges added since.
	ListBroader(ctx context.Context, projectID, listID, conceptID string) ([]domain.ConceptBroaderEdge, error)
	ListNarrower(ctx context.Context, conceptID string) ([]domain.ConceptBroaderEdge, error)

	// SearchConceptListSourceEntries searches the source vocabulary behind a
	// list. The bool reports a degraded vocabulary service.
	SearchConceptListSourceEntries(ctx context.Context, projectID, conceptListID, query, lang string, limit int) ([]ConceptListEntryView, bool, error)

	// RenderConceptListSKOS serialises a list to SKOS. An export leaves the
	// system, so it must serialise the scope it claims to be exporting.
	RenderConceptListSKOS(ctx context.Context, projectID, listID string, w io.Writer) error
}

// Store owns the writes and hands out readers.
//
// A mutation takes no scope: it always runs against the live tables, and
// keeping writes here rather than on the Reader makes that structural instead
// of conventional. There is no way to express "write to a release".
//
// The write methods return ids, not views. A view is a read, and reading one
// back after a write goes through At(auth.Draft()) like any other read — so
// the draft read is visible in the diff rather than implied by a write that
// happens to return rendered data.
type Store interface {
	// At returns the reader for scope, or an error when scope is the invalid
	// zero value — which means a caller forgot to name one. Failing here is
	// deliberate: a nil-safe default would reintroduce the silent draft read.
	At(scope auth.ReadScope) (Reader, error)

	// --- concept lists ---
	CreateConceptList(ctx context.Context, projectID, id string, in ConceptListInput) error
	UpdateConceptList(ctx context.Context, projectID, id string, in ConceptListInput) error
	DeleteConceptList(ctx context.Context, projectID, id string) error
	SetListClosed(ctx context.Context, projectID, listID string, closed bool) error

	// NextConceptListID reserves the next per-project list number from
	// weave_entity_counters. It is a write: the counter advances.
	NextConceptListID(ctx context.Context, projectID string) (string, error)

	// ConceptListOverrideRefCount reports how many field overrides reference
	// the list. DeleteConceptList's in-use guard needs it, and it is always a
	// question about the live working state — you cannot delete from a release.
	ConceptListOverrideRefCount(ctx context.Context, projectID, listID string) (int, error)

	// --- list membership ---
	AddConceptListEntry(ctx context.Context, projectID, listID, vocabularyEntryID, vocabularyEntryURI string) (string, error)
	UpdateConceptListEntry(ctx context.Context, projectID, listID, entryID string, customLabel domain.Translations) error
	RemoveConceptListEntry(ctx context.Context, projectID, listID, entryID string) error
	ReorderConceptListEntries(ctx context.Context, projectID, listID string, entryIDs []string) error

	// --- SKOS hierarchy ---
	AddBroader(ctx context.Context, projectID, listID string, edge domain.ConceptBroaderEdge) (domain.ConceptBroaderEdge, error)
	RemoveBroader(ctx context.Context, projectID, listID, conceptID, edgeID string) error

	// --- local vocabulary and terms ---
	EnsureLocalVocabulary(ctx context.Context, projectID string) (string, error)
	CreateLocalTerm(ctx context.Context, projectID, vocabularyID string, in CreateTermInput) (string, error)
	PersistConnectorEntry(ctx context.Context, vocabularyID, uri string, label domain.Translations) (string, error)
	PersistSelectedVocabularyEntry(ctx context.Context, vocabularyID, uri string, label domain.Translations) (string, error)
}
