package projectontologyversion

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for project-ontology-version links.
//
// The link row uses (project_id, ontology_version_id) as a composite
// primary key — there is no surrogate ULID. Methods that take a versionID
// always also take projectID so callers can't accidentally hit a row
// outside the requested project.
type Store interface {
	// --- CRUD ---

	List(ctx context.Context, projectID string) ([]*domain.ProjectOntologyVersion, error)

	// Get returns (nil, nil) when no row matches the (projectID, versionID)
	// pair — callers map to 404. Other errors propagate.
	Get(ctx context.Context, projectID, versionID string) (*domain.ProjectOntologyVersion, error)

	Create(ctx context.Context, link *domain.ProjectOntologyVersion) error

	// Update only persists is_primary and usage_notes. Other columns are
	// immutable through this method.
	Update(ctx context.Context, link *domain.ProjectOntologyVersion) error

	// Delete removes a link. Callers should run usage guard
	// (CountPathElementUsage) before invoking this — the store does not
	// block deletes that leave orphaned path elements.
	Delete(ctx context.Context, projectID, versionID string) error

	// --- Lifecycle ---

	// SetPrimary atomically clears any existing primary in the project and
	// marks (projectID, versionID) primary. Caller must ensure the row exists.
	SetPrimary(ctx context.Context, projectID, versionID string) error

	// --- Read-side composition ---

	// ListWithCounts returns each link with its path-element usage count
	// in the same project.
	ListWithCounts(ctx context.Context, projectID string) ([]domain.ProjectOntologyVersionWithCounts, error)

	// BundleForProject returns each linked ontology with its raw schema
	// content, for the X3ML <target> blocks and the ZIP download.
	BundleForProject(ctx context.Context, projectID string) ([]domain.OntologyBundleEntry, error)

	// BundleForVersions returns each ontology matching the supplied version
	// IDs with its raw schema content. Used to build inheritance-aware X3ML
	// zip and targets when versionIDs comes from
	// Projects().ResolvedOntologyVersions (own + inherited). Returns nil,
	// nil when versionIDs is empty.
	BundleForVersions(ctx context.Context, versionIDs []string) ([]domain.OntologyBundleEntry, error)

	// ListGrouped returns the project's OWN linked versions organised
	// into base + extensions groups. Inherited groups are composed at the
	// service layer using deps.Weave.Projects().ResolvedOntologyVersions.
	ListGrouped(ctx context.Context, projectID string) ([]domain.LinkedOntologyGroup, error)

	// --- Usage gates ---

	// CountPathElementUsage returns how many path elements in the project
	// reference the given ontology version. 0 means safe to unlink.
	CountPathElementUsage(ctx context.Context, projectID, versionID string) (int64, error)

	// SamplePathElementFields returns up to limit fields referencing the
	// version, suitable for surfacing in 409 payloads. limit is clamped to
	// at most 10 (a SQL-level cap).
	SamplePathElementFields(ctx context.Context, projectID, versionID string, limit int) ([]domain.FieldUsageSample, error)

	// OntologyUsage returns (classesUsed, propertiesUsed) — the count of
	// distinct qnames from the given ontology version that the project's
	// fields actually reference. Drives the "Deployed Ontologies" usage
	// figures on the project overview tab. Single round-trip per call.
	OntologyUsage(ctx context.Context, projectID, versionID string) (classesUsed, propertiesUsed int, err error)
}
