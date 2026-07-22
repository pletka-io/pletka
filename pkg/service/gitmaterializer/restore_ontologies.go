package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// VendoredOntologyImport carries everything an ontology importer needs to
// materialize a single vendored ontology snapshot into the database.
type VendoredOntologyImport struct {
	Manifest OntologyVendorManifest
	RootDir  string

	// ExternalBindings carries the project snapshot's effective prefix->namespace
	// bindings so the importer can resolve namespaces the vendored RDF
	// references by full URI (e.g. AAAo -> crm) but does not itself declare.
	// They come from the project manifest already on disk, so threading them
	// keeps restore reproducible from the git snapshot alone.
	ExternalBindings []VendoredOntologyExternalBinding
}

// VendoredOntologyExternalBinding is one prefix->namespace binding threaded
// into a vendored ontology import from the project snapshot's effective
// bindings. It is a primitive so gitmaterializer's restore seam does not
// depend on the ontology slice's types.
type VendoredOntologyExternalBinding struct {
	Prefix    string
	Namespace string
}

// VendoredOntologyImporter imports a vendored ontology snapshot into the
// database.
type VendoredOntologyImporter interface {
	// ImportVendoredOntology must be idempotent per manifest.Ontology.VersionID.
	ImportVendoredOntology(ctx context.Context, imp VendoredOntologyImport) error
}

// hydrateVendoredOntologies imports every ontology vendored by plan's
// snapshot that is not already present in the database. It verifies vendor
// checksums first and aborts before touching the database on any mismatch.
// When the snapshot vendors ontologies but no importer is wired, it fails
// loudly rather than silently skipping the import.
func (m *Materializer) hydrateVendoredOntologies(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Snapshot == nil {
		return nil
	}
	snapshot := plan.Snapshot

	if err := verifyVendorChecksums(snapshot); err != nil {
		return err
	}

	ontologies := snapshot.Vendor.Ontologies
	if len(ontologies) == 0 {
		return nil
	}
	if m.ontologyImporter == nil {
		return fmt.Errorf("restore: snapshot vendors ontologies but no ontology importer is wired")
	}

	// The project's effective namespace bindings (crm, aaao, …) are needed so a
	// vendored ontology that references an external namespace by full URI can
	// resolve it — the ontology's own prefix is not enough. They live in the
	// project manifest already loaded from the snapshot, keeping restore
	// git-only.
	var externalBindings []VendoredOntologyExternalBinding
	if snapshot.Manifest.Namespaces != nil {
		for _, b := range snapshot.Manifest.Namespaces.EffectiveBindings {
			if strings.TrimSpace(b.Prefix) == "" || strings.TrimSpace(b.Namespace) == "" {
				continue
			}
			externalBindings = append(externalBindings, VendoredOntologyExternalBinding{
				Prefix:    b.Prefix,
				Namespace: b.Namespace,
			})
		}
	}

	for _, dep := range ontologies {
		if dep.Snapshot == nil {
			continue
		}
		versionID := strings.TrimSpace(dep.Snapshot.Manifest.Ontology.VersionID)
		if versionID == "" {
			return fmt.Errorf("hydrate vendored ontologies: %s@%s missing ontology version id", dep.Module, dep.Version)
		}

		if _, err := m.queries.WeaveGetOntologyVersionByID(ctx, versionID); err == nil {
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("hydrate vendored ontologies: get ontology version %s: %w", versionID, err)
		}

		imp := VendoredOntologyImport{
			Manifest:         dep.Snapshot.Manifest,
			RootDir:          dep.Snapshot.RootDir,
			ExternalBindings: externalBindings,
		}
		if err := m.ontologyImporter.ImportVendoredOntology(ctx, imp); err != nil {
			return fmt.Errorf("hydrate vendored ontologies: import %s@%s: %w", dep.Module, dep.Version, err)
		}
	}

	return nil
}
