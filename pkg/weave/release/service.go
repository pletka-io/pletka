package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/releasedeps"
	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// semverRE matches MAJOR.MINOR.PATCH with an optional pre-release or
// build-metadata suffix per https://semver.org. The regex is permissive
// on the suffix shape (any [0-9A-Za-z.-]+) — strict enough to reject
// shorthand like "0.5" or "1.0" that produced opaque snapshot failures
// previously, loose enough to allow common pre-release labels like
// "1.0.0-rc.1" or "1.0.0+build.42" without re-implementing the full
// grammar.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

type ErrConflict struct {
	Message string
}

func (e *ErrConflict) Error() string { return e.Message }

type CreateInput struct {
	Version     string `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type permissionDenied struct {
	capability weaveauth.Capability
}

func (e *permissionDenied) Error() string {
	return fmt.Sprintf("forbidden: requires %s", e.capability)
}

type Service struct {
	hosts releasedescriptor.Hosts
	store Store
	// sanctioned pool holder: tx-owning service — see docs-oss/architecture/slices.md
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewService wires release storage, the transaction owner and capture configuration.
func NewService(store Store, pool *pgxpool.Pool, log *slog.Logger, options ...Option) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{store: store, pool: pool, log: log}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Service) ListByProject(ctx context.Context, projectID string) ([]Release, error) {
	if err := s.requireRead(ctx); err != nil {
		return nil, err
	}
	return s.store.ListByProject(ctx, projectID)
}

func (s *Service) Get(ctx context.Context, projectID, version string) (*Release, error) {
	if err := s.requireRead(ctx); err != nil {
		return nil, err
	}
	return s.store.Get(ctx, projectID, version)
}

func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*Release, error) {
	if err := s.requireEdit(ctx); err != nil {
		return nil, err
	}
	if errs := validateCreate(in); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	principal := weaveauth.PrincipalFromContext(ctx)
	if principal == nil || principal.ActorID == "" {
		return nil, errors.New("release: missing principal")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin release tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var draftCount int64
	if err := tx.QueryRow(ctx, countDraftSnapshotCandidatesSQL, projectID).Scan(&draftCount); err != nil {
		return nil, fmt.Errorf("count release snapshot candidates: %w", err)
	}
	if draftCount == 0 {
		return nil, &ErrValidation{Fields: map[string][]string{
			"version": {"cannot create a release for an empty project"},
		}}
	}
	blockingParents, err := findDraftParentDependencies(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("check draft parent dependencies: %w", err)
	}
	if len(blockingParents) > 0 {
		return nil, &ErrValidation{Fields: map[string][]string{
			"parent_dependencies": {fmt.Sprintf("cannot create a release while parent dependencies still follow draft or lack verified release archives: %s", strings.Join(blockingParents, ", "))},
		}}
	}

	var created Release
	err = tx.QueryRow(ctx, `
		INSERT INTO weave_releases (project_id, version, title, description, created_by_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING project_id, version, title, description, created_at, created_by_id
	`, projectID, in.Version, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), principal.ActorID).Scan(
		&created.ProjectID,
		&created.Version,
		&created.Title,
		&created.Description,
		&created.CreatedAt,
		&created.CreatedByID,
	)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.ConstraintName == "weave_releases_pkey" {
			return nil, &ErrConflict{Message: "release version already exists"}
		}
		return nil, fmt.Errorf("insert release metadata: %w", err)
	}

	if err := snapshotOwnedState(ctx, tx, projectID, in.Version); err != nil {
		return nil, fmt.Errorf("snapshot owned release state: %w", err)
	}
	for _, stmt := range snapshotStatements {
		if _, err := tx.Exec(ctx, stmt, projectID, in.Version); err != nil {
			return nil, fmt.Errorf("snapshot release state: %w", err)
		}
	}
	if err := releasedeps.Capture(ctx, sqlcgen.New(tx), projectID, in.Version); err != nil {
		var unavailable *releasedeps.UnavailableError
		if errors.As(err, &unavailable) {
			return nil, &ErrValidation{Fields: map[string][]string{"dependencies": {unavailable.Error()}}}
		}
		return nil, fmt.Errorf("capture release project dependencies: %w", err)
	}
	if err := ontologyrelease.CaptureProject(ctx, sqlcgen.New(tx), projectID, in.Version); err != nil {
		var invalid interface{ ValidationFields() map[string][]string }
		if errors.As(err, &invalid) {
			return nil, &ErrValidation{Fields: map[string][]string{"ontologies": {err.Error()}}}
		}
		return nil, fmt.Errorf("capture release ontology dependencies: %w", err)
	}
	if err := sqlcgen.New(tx).WeaveSnapshotPinnedAdoptions(ctx, sqlcgen.WeaveSnapshotPinnedAdoptionsParams{ProjectID: projectID, Version: in.Version}); err != nil {
		return nil, fmt.Errorf("capture pinned adoption receipts: %w", err)
	}

	if err := releasedescriptor.Capture(ctx, sqlcgen.New(tx), projectID, in.Version, s.hosts); err != nil {
		var missing *releasedescriptor.UnavailableError
		var invalid *releasedescriptor.InvalidError
		if errors.As(err, &missing) || errors.As(err, &invalid) {
			return nil, &ErrValidation{Fields: map[string][]string{"descriptors": {err.Error()}}}
		}
		return nil, fmt.Errorf("capture release descriptors: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO weave_change_set (project_id, actor_id, actor_name, actor_email,
			commit_message, started_at, closed_at, kind, release_version)
		VALUES ($1, $2, 'pletka-system', 'system@pletka.local', $3, NOW(), NOW(), 'release', $4)
	`, projectID, principal.ActorID,
		fmt.Sprintf("Release v%s", in.Version), in.Version); err != nil {
		return nil, fmt.Errorf("enqueue release change set: %w", err)
	}

	if err := sqlcgen.New(tx).WeaveMarkReleaseArchiveOwnership(ctx, sqlcgen.WeaveMarkReleaseArchiveOwnershipParams{ProjectID: projectID, Version: in.Version}); err != nil {
		return nil, fmt.Errorf("mark release archive ownership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit release tx: %w", err)
	}
	return &created, nil
}

func (s *Service) requireRead(ctx context.Context) error {
	snap := weaveauth.FromContext(ctx)
	res := weaveauth.ProjectResourceFromContext(ctx)
	if !snap.Can(weaveauth.ProjectRead, res, nil) {
		return &permissionDenied{capability: weaveauth.ProjectRead}
	}
	return nil
}

func (s *Service) requireEdit(ctx context.Context) error {
	snap := weaveauth.FromContext(ctx)
	res := weaveauth.ProjectResourceFromContext(ctx)
	if !snap.Can(weaveauth.ProjectEdit, res, nil) {
		return &permissionDenied{capability: weaveauth.ProjectEdit}
	}
	return nil
}

func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	version := strings.TrimSpace(in.Version)
	switch {
	case version == "":
		errs["version"] = append(errs["version"], "version is required")
	case !semverRE.MatchString(version):
		errs["version"] = append(errs["version"], "version must be in MAJOR.MINOR.PATCH form (for example 1.0.0)")
	}
	return errs
}

func findDraftParentDependencies(ctx context.Context, tx pgx.Tx, projectID string) ([]string, error) {
	return sqlcgen.New(tx).WeaveUnverifiedReleaseParents(ctx, projectID)
}

const countDraftSnapshotCandidatesSQL = `
	SELECT
		(SELECT COUNT(*) FROM weave_categories WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_fields WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_models WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_collections WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_field_overrides WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_project_ontology_versions WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_namespace_bindings WHERE project_id = $1)
`

var snapshotStatements = []string{
	`INSERT INTO weave_categories_archive (
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		status, project_id, canonical_order, deprecated, version_number
	)
	SELECT
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, canonical_order, deprecated, $2
	FROM weave_categories
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_fields_archive (
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		status, project_id, ontology_scope, ontology_path, path_elements,
		expected_value_type, examples, staging_id, deprecated, version_number, subfield_paths
	)
	SELECT
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, ontology_scope, ontology_path, path_elements,
		expected_value_type, examples, staging_id, deprecated, $2, subfield_paths
	FROM weave_fields
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_collections_archive (
		id, created_at, updated_at, system_name, ui_name, description,
		status, project_id, ontology_scope, collection_number,
		canonical_collection_order, staging_id, deprecated, default_category_id, version_number
	)
	SELECT
		id, created_at, updated_at, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, ontology_scope, collection_number,
		canonical_collection_order, staging_id, deprecated, default_category_id, $2
	FROM weave_collections
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_concept_lists_archive (
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		status, project_id, list_type, vocabulary_id, is_closed, version_number
	)
	SELECT
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, list_type, vocabulary_id, is_closed, $2
	FROM weave_concept_lists
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_concept_list_entries_archive (
		id, concept_list_id, vocabulary_entry_id, position, custom_label,
		created_at, updated_at, version_number
	)
	SELECT
		e.id, e.concept_list_id, e.vocabulary_entry_id, e.position, e.custom_label,
		e.created_at, e.updated_at, $2
	FROM weave_concept_list_entries e
	JOIN weave_concept_lists cl ON cl.id = e.concept_list_id
	WHERE cl.project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_override_refs_archive (
		override_id, ref_type, target_id, semantic_id, position, project_id, version_number
	)
	SELECT
		r.override_id, r.ref_type, r.target_id, r.semantic_id, r.position, o.project_id, $2
	FROM weave_override_refs r
	JOIN weave_field_overrides o ON o.id = r.override_id
	WHERE o.project_id = $1
	ON CONFLICT (override_id, ref_type, "position", version_number) DO NOTHING`,
	`INSERT INTO weave_entity_forks_archive (
		project_id, entity_type, fork_entity_id, source_project_id,
		source_entity_id, source_version, forked_at, created_by_id, version_number
	)
	SELECT
		project_id, entity_type, fork_entity_id, source_project_id,
		source_entity_id, source_version, forked_at, created_by_id, $2
	FROM weave_entity_forks
	WHERE project_id = $1
	ON CONFLICT (project_id, entity_type, fork_entity_id, version_number) DO NOTHING`,
	// weave_examples.status is a validation vocabulary (draft/valid/
	// has_issues), not the draft/published entity vocabulary the CASE WHEN
	// pattern elsewhere in this list normalizes — copied verbatim rather than
	// stamped 'published'.
	`INSERT INTO weave_examples_archive (
		id, project_id, entity_type, entity_id, title, description, status,
		created_at, updated_at, version_number
	)
	SELECT
		id, project_id, entity_type, entity_id, title, description, status,
		created_at, updated_at, $2
	FROM weave_examples
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	// Values follow their example, so the project filter is a join rather
	// than a column on this table.
	`INSERT INTO weave_example_values_archive (
		id, example_id, override_id, field_id, part_of_collection_id,
		occurrence_index, value_kind, value_payload, text_value, number_value,
		date_value, uri_value, concept_uri, linked_example_id, slot_path,
		created_at, updated_at, version_number
	)
	SELECT
		v.id, v.example_id, v.override_id, v.field_id, v.part_of_collection_id,
		v.occurrence_index, v.value_kind, v.value_payload, v.text_value, v.number_value,
		v.date_value, v.uri_value, v.concept_uri, v.linked_example_id, v.slot_path,
		v.created_at, v.updated_at, $2
	FROM weave_example_values v
	JOIN weave_examples e ON e.id = v.example_id
	WHERE e.project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_vocabularies_archive (
		id, project_id, semantic_id, system_name, ui_name, description,
		status, connector_type, base_uri, config, deprecated,
		created_at, updated_at, version_number
	)
	SELECT
		id, project_id, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END,
		connector_type, base_uri, config, deprecated,
		created_at, updated_at, $2
	FROM weave_vocabularies
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	// Entries follow their vocabulary, so the project filter is a join rather
	// than a column on this table — the same shape as example values.
	`INSERT INTO weave_vocabulary_entries_archive (
		id, vocabulary_id, uri, label, scope_note, broader_uri,
		broader_path, broader_path_items, external_id,
		created_at, updated_at, version_number
	)
	SELECT
		e.id, e.vocabulary_id, e.uri, e.label, e.scope_note, e.broader_uri,
		e.broader_path, e.broader_path_items, e.external_id,
		e.created_at, e.updated_at, $2
	FROM weave_vocabulary_entries e
	JOIN weave_vocabularies v ON v.id = e.vocabulary_id
	WHERE v.project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	// This table carries its own project_id, so the filter is direct rather
	// than a join. The live unique key is (model_id, category_id,
	// collection_id), not (model_id, version_number) — that composite is what
	// allows more than one collection in the same category on the same model,
	// so the archive keeps the bigserial id as its key half.
	`INSERT INTO weave_collection_placements_archive (
		id, project_id, model_id, category_id, collection_id,
		is_required, min_occurs, max_occurs, is_hidden,
		created_at, updated_at, version_number
	)
	SELECT
		id, project_id, model_id, category_id, collection_id,
		is_required, min_occurs, max_occurs, is_hidden,
		created_at, updated_at, $2
	FROM weave_collection_placements
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	// A release is citable, so who was credited at that release is part of
	// what is cited. Neither live table has an id — see the archive
	// migration's comment for the composite-key deviation this deliberately
	// takes instead of inventing a surrogate id. The ON CONFLICT targets are
	// the live primary keys plus version_number, in full: dropping "position"
	// or "role" would let a legitimately distinct live row collide and
	// silently vanish from the snapshot.
	`INSERT INTO weave_project_attributions_archive (
		project_id, actor_id, kind, "position", note, created_at, version_number
	)
	SELECT
		project_id, actor_id, kind, "position", note, created_at, $2
	FROM weave_project_attributions
	WHERE project_id = $1
	ON CONFLICT (project_id, actor_id, kind, "position", version_number) DO NOTHING`,
	`INSERT INTO weave_project_actors_archive (
		project_id, actor_id, role, created_at, version_number
	)
	SELECT
		project_id, actor_id, role, created_at, $2
	FROM weave_project_actors
	WHERE project_id = $1
	ON CONFLICT (project_id, actor_id, role, version_number) DO NOTHING`,
}
