package gitmaterializer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/jackc/pgx/v5"
)

func (m *Materializer) HydrateProjectShell(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Snapshot == nil {
		return fmt.Errorf("hydrate project shell: missing snapshot")
	}
	snapshot := plan.Snapshot
	projectID := strings.TrimSpace(snapshot.Manifest.Project.ID)
	if projectID == "" {
		return fmt.Errorf("hydrate project shell: missing project id")
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("hydrate project shell: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := m.queries.WithTx(tx)
	if err := m.hydrateProjectManifestTx(ctx, tx, q, snapshot.Manifest); err != nil {
		return err
	}
	if err := m.hydrateProjectInheritanceTx(ctx, tx, snapshot.Manifest); err != nil {
		return err
	}
	if err := m.hydrateProjectOntologiesTx(ctx, tx, q, snapshot.Manifest); err != nil {
		return err
	}
	if err := m.hydrateProjectNamespacesTx(ctx, tx, q, snapshot.Manifest); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("hydrate project shell: commit tx: %w", err)
	}
	return nil
}

func (m *Materializer) hydrateProjectManifestTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, manifest ProjectManifestFile) error {
	project := manifest.Project
	projectID := strings.TrimSpace(project.ID)

	if err := ensureManifestActorExists(ctx, q, project.Owner, "owner"); err != nil {
		return fmt.Errorf("hydrate project manifest: %w", err)
	}
	if err := ensureManifestActorExists(ctx, q, project.CreatedBy, "created_by"); err != nil {
		return fmt.Errorf("hydrate project manifest: %w", err)
	}

	uiName, err := marshalManifestTranslations(project.Title)
	if err != nil {
		return fmt.Errorf("hydrate project manifest: marshal title: %w", err)
	}
	description, err := marshalManifestTranslations(project.Description)
	if err != nil {
		return fmt.Errorf("hydrate project manifest: marshal description: %w", err)
	}
	readme, err := marshalManifestTranslations(project.Readme)
	if err != nil {
		return fmt.Errorf("hydrate project manifest: marshal readme: %w", err)
	}
	if readme == nil {
		readme = []byte(`{}`)
	}

	parentProjectID := primaryParentPointer(manifest.Inheritance)
	ownerID := actorIDFromManifest(project.Owner)
	createdByID := actorIDPtrFromManifest(project.CreatedBy)
	topics := project.Topics
	if topics == nil {
		topics = []string{}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO weave_projects (
			id, system_name, ui_name, description, status, namespace,
			parent_project_id, owner_id, staging_id, visibility, created_by_id,
			license, readme, topics, base_url, created_at, updated_at
		) VALUES (
			$1, NULL, $2, $3, 'draft', $4,
			$5, $6, NULL, $7, $8,
			$9, $10, $11, $12, NOW(), NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			ui_name = EXCLUDED.ui_name,
			description = EXCLUDED.description,
			namespace = EXCLUDED.namespace,
			parent_project_id = EXCLUDED.parent_project_id,
			owner_id = EXCLUDED.owner_id,
			visibility = EXCLUDED.visibility,
			created_by_id = EXCLUDED.created_by_id,
			license = EXCLUDED.license,
			readme = EXCLUDED.readme,
			topics = EXCLUDED.topics,
			base_url = EXCLUDED.base_url,
			updated_at = NOW()
	`, projectID, uiName, description, nullableString(project.Namespace), parentProjectID, ownerID, project.Visibility, createdByID, project.License, readme, topics, project.BaseURL)
	if err != nil {
		return fmt.Errorf("hydrate project manifest: upsert project %s: %w", projectID, err)
	}

	return nil
}

func (m *Materializer) hydrateProjectInheritanceTx(ctx context.Context, tx pgx.Tx, manifest ProjectManifestFile) error {
	projectID := strings.TrimSpace(manifest.Project.ID)
	if _, err := tx.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("hydrate project inheritance: clear project inheritances: %w", err)
	}
	if manifest.Inheritance == nil || len(manifest.Inheritance.Parents) == 0 {
		return nil
	}

	parents := append([]ProjectManifestParent(nil), manifest.Inheritance.Parents...)
	primaryParentID := ""
	for _, parent := range parents {
		if parent.IsPrimary {
			primaryParentID = strings.TrimSpace(parent.ProjectID)
			break
		}
	}
	if primaryParentID == "" && len(parents) > 0 {
		primaryParentID = strings.TrimSpace(parents[0].ProjectID)
	}
	for i, parent := range parents {
		parentID := strings.TrimSpace(parent.ProjectID)
		if parentID == "" {
			return fmt.Errorf("hydrate project inheritance: parent at index %d missing project_id", i)
		}
		sourceMode := strings.TrimSpace(parent.SourceMode)
		if sourceMode == "" {
			sourceMode = "draft"
		}
		sourceVersion := strings.TrimSpace(parent.SourceVersion)
		switch sourceMode {
		case "draft":
			sourceVersion = ""
		case "release":
			if sourceVersion == "" {
				return fmt.Errorf("hydrate project inheritance: parent %s uses release mode without source_version", parentID)
			}
		default:
			return fmt.Errorf("hydrate project inheritance: parent %s has invalid source_mode %q", parentID, sourceMode)
		}
		if _, err := m.queries.WithTx(tx).WeaveGetProjectByID(ctx, parentID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("hydrate project inheritance: parent project %s not found", parentID)
			}
			return fmt.Errorf("hydrate project inheritance: get parent project %s: %w", parentID, err)
		}
		isPrimary := parentID == primaryParentID
		if _, err := tx.Exec(ctx, `
			INSERT INTO weave_project_inheritance (
				project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode, source_version
			) VALUES ($1, $2, $3, $4, NOW(), $5, $6)
		`, projectID, parentID, isPrimary, parent.CanonicalOrder, sourceMode, nullableString(sourceVersion)); err != nil {
			return fmt.Errorf("hydrate project inheritance: insert parent %s: %w", parentID, err)
		}
	}
	return nil
}

func (m *Materializer) hydrateProjectOntologiesTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, manifest ProjectManifestFile) error {
	projectID := strings.TrimSpace(manifest.Project.ID)
	if _, err := tx.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("hydrate project ontologies: clear project links: %w", err)
	}
	if manifest.Ontologies == nil {
		return nil
	}

	addedByID := actorIDPtrFromManifest(manifest.Project.CreatedBy)
	for _, link := range manifest.Ontologies.LinkedVersions {
		versionID := strings.TrimSpace(link.OntologyVersionID)
		if versionID == "" {
			row, err := q.WeaveGetOntologyVersionByOntologyAndString(ctx, sqlcgen.WeaveGetOntologyVersionByOntologyAndStringParams{
				OntologyID:    link.OntologyID,
				VersionString: link.Version,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("hydrate project ontologies: ontology version %s@%s not found", link.OntologyID, link.Version)
				}
				return fmt.Errorf("hydrate project ontologies: resolve ontology version %s@%s: %w", link.OntologyID, link.Version, err)
			}
			versionID = row.ID
		} else {
			if _, err := q.WeaveGetOntologyVersionByID(ctx, versionID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("hydrate project ontologies: ontology version %s not found", versionID)
				}
				return fmt.Errorf("hydrate project ontologies: get ontology version %s: %w", versionID, err)
			}
		}

		isPrimary := link.IsPrimary
		usageNotes := ""
		if _, err := q.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
			ProjectID:         projectID,
			OntologyVersionID: versionID,
			AddedByID:         addedByID,
			IsPrimary:         &isPrimary,
			UsageNotes:        &usageNotes,
		}); err != nil {
			return fmt.Errorf("hydrate project ontologies: create link %s: %w", versionID, err)
		}
	}
	return nil
}

func (m *Materializer) hydrateProjectNamespacesTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, manifest ProjectManifestFile) error {
	projectID := strings.TrimSpace(manifest.Project.ID)
	if _, err := tx.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1 AND source = 'user'`, projectID); err != nil {
		return fmt.Errorf("hydrate project namespaces: clear user bindings: %w", err)
	}
	if manifest.Namespaces == nil {
		return nil
	}
	for i, binding := range manifest.Namespaces.EffectiveBindings {
		if strings.TrimSpace(binding.Prefix) == "" || strings.TrimSpace(binding.Namespace) == "" {
			return fmt.Errorf("hydrate project namespaces: binding at index %d missing prefix or namespace", i)
		}
		exists, err := q.WeavePrefixNamespaceBindingExists(ctx, sqlcgen.WeavePrefixNamespaceBindingExistsParams{
			Prefix:    binding.Prefix,
			Namespace: binding.Namespace,
			ID:        "",
		})
		if err != nil {
			return fmt.Errorf("hydrate project namespaces: check binding %s -> %s: %w", binding.Prefix, binding.Namespace, err)
		}
		if exists {
			continue
		}
		if _, err := q.WeaveCreateUserNamespaceBinding(ctx, sqlcgen.WeaveCreateUserNamespaceBindingParams{
			ID:        ids.GenerateULID(),
			ProjectID: stringPtr(projectID),
			Prefix:    binding.Prefix,
			Namespace: binding.Namespace,
			Weight:    int64(i),
		}); err != nil {
			return fmt.Errorf("hydrate project namespaces: create binding %s -> %s: %w", binding.Prefix, binding.Namespace, err)
		}
	}
	return nil
}

func marshalManifestTranslations(t domain.Translations) ([]byte, error) {
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}

func ensureManifestActorExists(ctx context.Context, q *sqlcgen.Queries, actor *manifestActor, label string) error {
	if actor == nil || strings.TrimSpace(actor.ActorID) == "" {
		return nil
	}
	if _, err := q.WeaveGetActorByID(ctx, actor.ActorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%s actor %s not found", label, actor.ActorID)
		}
		return fmt.Errorf("get %s actor %s: %w", label, actor.ActorID, err)
	}
	return nil
}

func actorIDFromManifest(actor *manifestActor) string {
	if actor == nil {
		return ""
	}
	return strings.TrimSpace(actor.ActorID)
}

func actorIDPtrFromManifest(actor *manifestActor) *string {
	id := actorIDFromManifest(actor)
	if id == "" {
		return nil
	}
	return &id
}

func nullableString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func primaryParentPointer(inheritance *ProjectManifestInheritance) *string {
	if inheritance == nil || len(inheritance.Parents) == 0 {
		return nil
	}
	for _, parent := range inheritance.Parents {
		if parent.IsPrimary {
			return nullableString(parent.ProjectID)
		}
	}
	return nullableString(inheritance.Parents[0].ProjectID)
}

func stringPtr(v string) *string {
	return &v
}
