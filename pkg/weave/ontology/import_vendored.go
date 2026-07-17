package ontology

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// VendoredOntologyImportRequest carries everything a vendored ontology
// snapshot knows, as primitives. It deliberately avoids depending on
// gitmaterializer types — pkg/app (Task 4) adapts a gitmaterializer
// manifest into this shape, so the ontology slice never imports
// gitmaterializer and gitmaterializer never imports this package's
// concrete Service.
type VendoredOntologyImportRequest struct {
	OntologyID    string // weave_ontologies.id to ensure/import under
	VersionID     string // weave_ontology_versions.id (idempotency key)
	VersionString string
	Slug          string // ontology prefix/slug
	Title         string
	Kind          string // ontology type (base/extension); mapped to the domain constant
	Namespace     string
	Prefixes      []string
	BaseDir       string   // directory containing the RDF source files
	Files         []string // RDF filenames relative to BaseDir (first = primary)
	Imports       []string // imported ontology modules (metadata only; not yet persisted)
}

// ImportVendoredVersion imports one vendored ontology version snapshot.
// It is idempotent per req.VersionID and reuses the existing file-based
// import pipeline (BuildImportVersionInputFromFile +
// ImportVersionWithOptions) rather than reimplementing RDF parsing.
func (s *Service) ImportVendoredVersion(ctx context.Context, req VendoredOntologyImportRequest) error {
	if err := validateVendoredFiles(req.Files); err != nil {
		return fmt.Errorf("import vendored ontology %s@%s: %w", req.Slug, req.VersionString, err)
	}

	existing, err := s.store.GetVersion(ctx, req.VersionID)
	if err != nil {
		return fmt.Errorf("import vendored ontology %s@%s: check existing version: %w", req.Slug, req.VersionString, err)
	}
	if existing != nil {
		return nil
	}

	ont, err := s.ensureVendoredOntology(ctx, req)
	if err != nil {
		return fmt.Errorf("import vendored ontology %s@%s: %w", req.Slug, req.VersionString, err)
	}

	if len(req.Files) == 0 {
		return fmt.Errorf("import vendored ontology %s@%s: no source files", req.Slug, req.VersionString)
	}

	built, err := BuildImportVersionInputFromFile(ImportVersionFileOptions{
		BaseDir:           req.BaseDir,
		File:              req.Files[0],
		Ontology:          ont,
		VersionID:         req.VersionID,
		VersionString:     req.VersionString,
		NamespaceBindings: vendoredNamespaceBindings(req),
	})
	if err != nil {
		return fmt.Errorf("import vendored ontology %s@%s: %w", req.Slug, req.VersionString, err)
	}

	if _, err := s.ImportVersionWithOptions(ctx, built.Input, ImportVersionOptions{
		VersionNamespaceBinding: vendoredVersionNamespaceBinding(req),
	}); err != nil {
		return fmt.Errorf("import vendored ontology %s@%s: %w", req.Slug, req.VersionString, err)
	}
	return nil
}

// validateVendoredFiles rejects manifest-supplied source filenames that are
// absolute or attempt to escape the ontology's base directory via ".."
// path segments. req.Files comes straight off an untrusted vendored
// manifest on disk, so every entry must resolve strictly inside req.BaseDir
// before it is ever joined into a path.
func validateVendoredFiles(files []string) error {
	for _, file := range files {
		if filepath.IsAbs(file) {
			return fmt.Errorf("vendored ontology file %q must be a relative path", file)
		}
		cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file)))
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return fmt.Errorf("vendored ontology file %q must not escape its base directory", file)
		}
	}
	return nil
}

// ensureVendoredOntology looks up req.OntologyID and, when missing,
// creates a minimal ontology row from the request's primitives.
//
// Defaults documented here (per spec: prefer sensible minimal defaults
// over inventing new manifest requirements):
//   - FamilyID is left nil — vendored ontologies land unassigned to a
//     family; curators assign one later via the admin UI.
//   - Prefix falls back to req.Slug when req.Prefixes is empty, since
//     CreateOntologyInput.Prefix is required.
//   - Description is left as the zero Translations value.
func (s *Service) ensureVendoredOntology(ctx context.Context, req VendoredOntologyImportRequest) (*domain.Ontology, error) {
	existing, err := s.store.GetOntology(ctx, req.OntologyID)
	if err != nil {
		return nil, fmt.Errorf("check existing ontology: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	prefix := strings.TrimSpace(req.Slug)
	if len(req.Prefixes) > 0 && strings.TrimSpace(req.Prefixes[0]) != "" {
		prefix = strings.TrimSpace(req.Prefixes[0])
	}

	created, err := s.CreateOntology(ctx, CreateOntologyInput{
		ID:           req.OntologyID,
		Prefix:       prefix,
		Namespace:    req.Namespace,
		Name:         req.Title,
		OntologyType: vendoredOntologyKind(req.Kind),
	})
	if err != nil {
		return nil, fmt.Errorf("create ontology: %w", err)
	}
	return created, nil
}

// vendoredOntologyKind maps the manifest's free-text kind string to the
// domain constant, defaulting to OntologyTypeBase for empty/unrecognized
// values so an unexpected manifest value never fails the import.
func vendoredOntologyKind(kind string) domain.OntologyType {
	if domain.OntologyType(strings.ToLower(strings.TrimSpace(kind))) == domain.OntologyTypeExtension {
		return domain.OntologyTypeExtension
	}
	return domain.OntologyTypeBase
}

// vendoredNamespaceBindings builds one NamespaceBinding per req.Prefixes
// entry, all bound to req.Namespace — mirroring how
// gitmaterializer.writeVendoredOntology always writes Prefixes as
// []string{ontology.Prefix} for the ontology's own namespace.
func vendoredNamespaceBindings(req VendoredOntologyImportRequest) []NamespaceBinding {
	if req.Namespace == "" {
		return nil
	}
	bindings := make([]NamespaceBinding, 0, len(req.Prefixes))
	for _, prefix := range req.Prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		bindings = append(bindings, NamespaceBinding{Prefix: prefix, Namespace: req.Namespace})
	}
	return bindings
}

// vendoredVersionNamespaceBinding returns the single version-derived
// namespace binding ImportVersionWithOptions persists alongside the
// imported rows (see ImportVersionOptions.VersionNamespaceBinding). Uses
// the first prefix as primary; returns nil when no prefix is available,
// since the binding is optional metadata, not required for import to
// succeed.
func vendoredVersionNamespaceBinding(req VendoredOntologyImportRequest) *NamespaceBinding {
	if req.Namespace == "" || len(req.Prefixes) == 0 {
		return nil
	}
	prefix := strings.TrimSpace(req.Prefixes[0])
	if prefix == "" {
		return nil
	}
	return &NamespaceBinding{Prefix: prefix, Namespace: req.Namespace}
}
