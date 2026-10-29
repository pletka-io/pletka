package ontologysnapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// ValidateBundle validates all artifacts and their shared body/context identities
// without database access. Restore orchestrators must call it during preflight,
// before committing independently restored dependencies or project content.
func ValidateBundle(artifacts []Artifact) error {
	_, _, err := restoreValues(artifacts)
	return err
}

// Restore installs exact sealed bodies and retains the original imported
// artifacts in one transaction. Imported evidence is separate from locally
// captured snapshots: it creates no project release, completion certificate,
// ontology publication or active selection. No RDF is reparsed.
//
// Existing seals and imports are reusable only with identical identities and
// bytes. An unsealed version is never overwritten or implicitly certified.
func Restore(ctx context.Context, pool *pgxpool.Pool, artifacts []Artifact) (err error) {
	bodies, evidence, err := restoreValues(artifacts)
	if err != nil || len(evidence) == 0 {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	defer func() {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && (pgerr.ConstraintName == "ontology_artifact_conflict" || pgerr.ConstraintName == "legacy_ontology_import_conflict") {
			err = &restoreConflict{cause: err}
		}
	}()
	q := sqlcgen.New(tx)
	documents := make([]string, len(evidence))
	for i, a := range evidence {
		documents[i] = a.JSON
	}
	if err := q.WeaveLockImportedLegacyOntologyContexts(ctx, documents); err != nil {
		return err
	}
	for _, b := range bodies {
		if err := q.WeaveRestoreOntologyBody(ctx, sqlcgen.WeaveRestoreOntologyBodyParams{
			OntologyID: b.OntologyID, VersionID: b.VersionID, BodyText: b.BodyJSON, Digest: b.Digest,
		}); err != nil {
			return err
		}
	}
	for _, a := range evidence {
		if err := q.WeaveRetainImportedLegacyOntologySnapshot(ctx, sqlcgen.WeaveRetainImportedLegacyOntologySnapshotParams{
			Document: a.JSON, Digest: a.Digest,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type restoreConflict struct{ cause error }

func (e *restoreConflict) Error() string {
	return "legacy ontology restore conflicts with retained content: " + e.cause.Error()
}
func (e *restoreConflict) Unwrap() error           { return e.cause }
func (e *restoreConflict) ConflictMessage() string { return e.Error() }

// Validate every artifact before opening the transaction. Keep inventories
// keyed by concrete identity, not owner or version label; source selections
// remain untouched inside each original artifact. Sort all body locks globally.
func restoreValues(artifacts []Artifact) ([]Body, []Artifact, error) {
	bodies := map[string]Body{}
	evidence := map[Origin]Artifact{}
	contextValues := map[Origin]string{}
	sourceValues := map[Origin]Source{}
	for _, a := range artifacts {
		d, err := a.Decode()
		if err != nil {
			return nil, nil, err
		}
		o := Origin{project: d.ProjectID, version: d.Version}
		if old, exists := evidence[o]; exists && old != a {
			return nil, nil, fmt.Errorf("conflicting imported snapshots for %s@%s", d.ProjectID, d.Version)
		}
		evidence[o] = a
		contexts, err := restoreContexts(d, sourceValues, contextValues)
		if err != nil {
			return nil, nil, err
		}
		for _, context := range contexts {
			for _, b := range context.Inventory {
				if old, exists := bodies[b.VersionID]; exists && (old.OntologyID != b.OntologyID || old.Digest != b.Digest || old.BodyJSON != b.BodyJSON) {
					return nil, nil, fmt.Errorf("conflicting imported bodies for %s", b.VersionID)
				}
				bodies[b.VersionID] = b
			}
		}
	}
	orderedBodies := make([]Body, 0, len(bodies))
	for _, b := range bodies {
		orderedBodies = append(orderedBodies, b)
	}
	slices.SortFunc(orderedBodies, func(a, b Body) int { return strings.Compare(a.VersionID, b.VersionID) })
	origins := make([]Origin, 0, len(evidence))
	for o := range evidence {
		origins = append(origins, o)
	}
	slices.SortFunc(origins, func(a, b Origin) int {
		if c := strings.Compare(a.project, b.project); c != 0 {
			return c
		}
		return strings.Compare(a.version, b.version)
	})
	orderedEvidence := make([]Artifact, 0, len(origins))
	for _, o := range origins {
		orderedEvidence = append(orderedEvidence, evidence[o])
	}
	return orderedBodies, orderedEvidence, nil
}

// Collect the root and source contexts, checking their identities against all
// previously collected bundle contexts. This operates only on detached values.
func restoreContexts(d Document, sourceValues map[Origin]Source, contextValues map[Origin]string) ([]Document, error) {
	contexts := []Document{d}
	for _, source := range d.Sources {
		s, err := decodeDocument(source.JSON, source.Digest)
		if err != nil {
			return nil, err
		}
		o := Origin{project: s.ProjectID, version: s.Version}
		if old, exists := sourceValues[o]; exists && old != source {
			return nil, fmt.Errorf("conflicting imported source contexts for %s@%s", s.ProjectID, s.Version)
		}
		sourceValues[o] = source
		contexts = append(contexts, s)
	}
	for _, context := range contexts {
		// A root and an embedded source may name the same local context.
		// Compare its value without the enclosing flat source closure; the
		// source-to-source comparison above additionally requires exact bytes.
		o := Origin{project: context.ProjectID, version: context.Version}
		context.Sources = nil
		data, err := json.Marshal(context)
		if err != nil {
			return nil, err
		}
		if old, exists := contextValues[o]; exists && old != string(data) {
			return nil, fmt.Errorf("conflicting imported local contexts for %s@%s", context.ProjectID, context.Version)
		}
		contextValues[o] = string(data)
	}
	return contexts, nil
}
