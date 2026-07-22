package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

type InitProjectOptions struct {
	SelfContained bool
}

type vendorState struct {
	projects   map[string]bool
	ontologies map[string]bool
}

func newVendorState() *vendorState {
	return &vendorState{
		projects:   make(map[string]bool),
		ontologies: make(map[string]bool),
	}
}

func (m *Materializer) writeProjectTree(ctx context.Context, workDir, projectID string) error {
	if err := m.writeProjectManifest(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeAdoptionManifests(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeForkManifests(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writePletkaMod(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeCategories(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeFields(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeModels(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeCollections(ctx, workDir, projectID); err != nil {
		return err
	}
	if err := m.writeScopedOverridesByProject(ctx, workDir, projectID, "model"); err != nil {
		return err
	}
	if err := m.writeScopedOverridesByProject(ctx, workDir, projectID, "collection"); err != nil {
		return err
	}
	if err := m.writeAdoptedEntities(ctx, workDir, projectID); err != nil {
		return err
	}
	return nil
}

func (m *Materializer) writeVendorSnapshot(ctx context.Context, workDir, projectID string) error {
	state := newVendorState()
	entries, err := m.vendorDependenciesForProject(ctx, workDir, projectID, "", state)
	if err != nil {
		return err
	}
	return writePletkaSum(workDir, entries)
}

func (m *Materializer) vendorDependenciesForProject(ctx context.Context, rootDir, projectID, version string, state *vendorState) ([]pletkaSumEntry, error) {
	var entries []pletkaSumEntry

	var (
		projectReqs []pletkaProjectRequirement
		err         error
	)
	if strings.TrimSpace(version) == "" || strings.TrimSpace(version) == parentDependencyVersionDraft {
		projectReqs, err = m.loadPletkaProjectRequirements(ctx, projectID)
	} else {
		projectReqs, err = m.loadPletkaProjectRequirementsAtVersion(ctx, projectID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("load vendored project requirements for %s@%s: %w", projectID, strings.TrimSpace(version), err)
	}
	for _, req := range projectReqs {
		projectKey := req.Module + "@" + strings.TrimSpace(req.Version)
		if state.projects[projectKey] {
			continue
		}
		state.projects[projectKey] = true
		vendorDir := vendoredProjectDir(rootDir, req)
		if err := os.MkdirAll(vendorDir, 0o755); err != nil {
			return nil, fmt.Errorf("create vendored project dir: %w", err)
		}
		if strings.TrimSpace(req.Version) == "" || strings.TrimSpace(req.Version) == parentDependencyVersionDraft {
			if err := m.writeProjectTree(ctx, vendorDir, req.ProjectID); err != nil {
				return nil, fmt.Errorf("write vendored project %s: %w", req.ProjectID, err)
			}
		} else {
			if err := m.writeProjectTreeVersion(ctx, vendorDir, req.ProjectID, req.Version); err != nil {
				return nil, fmt.Errorf("write vendored project %s@%s: %w", req.ProjectID, req.Version, err)
			}
		}
		treeHash, err := hashDirectoryTree(vendorDir)
		if err != nil {
			return nil, fmt.Errorf("hash vendored project %s: %w", req.ProjectID, err)
		}
		entries = append(entries, pletkaSumEntry{
			Module:  req.Module,
			Version: req.Version,
			TreeSHA: treeHash,
		})
		childEntries, err := m.vendorDependenciesForProject(ctx, rootDir, req.ProjectID, req.Version, state)
		if err != nil {
			return nil, err
		}
		entries = append(entries, childEntries...)
	}

	var ontologyReqs []pletkaOntologyRequirement
	if strings.TrimSpace(version) == "" || strings.TrimSpace(version) == parentDependencyVersionDraft {
		ontologyReqs, err = m.loadPletkaOntologyRequirements(ctx, projectID)
	} else {
		ontologyReqs, err = m.loadPletkaOntologyRequirementsAtVersion(ctx, projectID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("load vendored ontology requirements for %s@%s: %w", projectID, strings.TrimSpace(version), err)
	}
	for _, req := range ontologyReqs {
		key := req.OntologyVersionID
		if key == "" {
			key = req.Module + "@" + req.Version
		}
		if state.ontologies[key] {
			continue
		}
		state.ontologies[key] = true
		version, err := m.queries.WeaveGetOntologyVersionByID(ctx, req.OntologyVersionID)
		if err != nil {
			return nil, fmt.Errorf("get vendored ontology version %s: %w", req.OntologyVersionID, err)
		}
		ontology, err := m.queries.WeaveGetOntologyByID(ctx, version.OntologyID)
		if err != nil {
			return nil, fmt.Errorf("get vendored ontology %s: %w", version.OntologyID, err)
		}
		vendorDir := filepath.Join(rootDir, "vendor", "ontologies", filepath.FromSlash(req.Module), vendorVersionDir(version.VersionString, version.ID))
		if err := m.writeVendoredOntology(ctx, vendorDir, ontology, version); err != nil {
			return nil, err
		}
		treeHash, err := hashDirectoryTree(vendorDir)
		if err != nil {
			return nil, fmt.Errorf("hash vendored ontology %s@%s: %w", req.Module, req.Version, err)
		}
		entries = append(entries, pletkaSumEntry{
			Module:  req.Module,
			Version: req.Version,
			TreeSHA: treeHash,
		})
	}

	return entries, nil
}

func vendoredProjectDir(rootDir string, req pletkaProjectRequirement) string {
	base := filepath.Join(rootDir, "vendor", "projects", filepath.FromSlash(req.Module))
	version := strings.TrimSpace(req.Version)
	if version == "" || version == parentDependencyVersionDraft {
		return base
	}
	return filepath.Join(base, version)
}

type ontologyVendorManifest struct {
	SchemaVersion int                        `json:"schema_version"`
	Ontology      ontologyVendorManifestRoot `json:"ontology"`
	Sources       *ontologyVendorSources     `json:"sources,omitempty"`
	Imports       []string                   `json:"imports,omitempty"`
}

type ontologyVendorManifestRoot struct {
	Module     string   `json:"module"`
	OntologyID string   `json:"ontology_id"`
	VersionID  string   `json:"version_id"`
	Version    string   `json:"version"`
	Slug       string   `json:"slug"`
	Title      string   `json:"title,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	// NamespaceAliases lists every additional namespace URI the prefix is
	// globally bound to — the URI variants this ontology's serializations
	// answer to beyond the canonical Namespace (e.g. crmgeo's
	// ics.forth.gr base alongside the registry namespace). Restore binds
	// the prefix to each alias so a vendored RDF that uses a variant URI
	// still resolves from the git snapshot alone.
	NamespaceAliases []string `json:"namespace_aliases,omitempty"`
	Prefixes         []string `json:"prefixes,omitempty"`
	SourceURL        string   `json:"source_url,omitempty"`
}

type ontologyVendorSources struct {
	Files []string `json:"files,omitempty"`
}

func (m *Materializer) writeVendoredOntology(ctx context.Context, workDir string, ontology sqlcgen.WeaveOntology, version sqlcgen.WeaveOntologyVersion) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create vendored ontology dir: %w", err)
	}
	moduleSlug := ontologyModuleSlug(ontology.Prefix, ontology.Name, ontology.ID)

	var sourceFiles []string
	if version.RdfContent == nil || strings.TrimSpace(*version.RdfContent) == "" {
		return fmt.Errorf("vendored ontology %s@%s is missing rdf_content; self-contained snapshots require source-complete ontology artifacts", ontology.ID, version.VersionString)
	}
	filename := strings.TrimSpace(derefStr(version.OriginalFilename))
	if filename == "" {
		filename = ontology.ID + ".ttl"
	}
	rel := filepath.ToSlash(filepath.Join("src", filename))
	if err := writeEntityFile(workDir, rel, []byte(*version.RdfContent)); err != nil {
		return err
	}
	sourceFiles = append(sourceFiles, rel)

	aliases, err := m.vendoredNamespaceAliases(ctx, ontology.Prefix, ontology.Namespace)
	if err != nil {
		return err
	}

	manifest := ontologyVendorManifest{
		SchemaVersion: 1,
		Ontology: ontologyVendorManifestRoot{
			Module:           m.ontologyModulePath(moduleSlug),
			OntologyID:       ontology.ID,
			VersionID:        version.ID,
			Version:          version.VersionString,
			Slug:             moduleSlug,
			Title:            ontology.Name,
			Kind:             ontology.OntologyType,
			Namespace:        ontology.Namespace,
			NamespaceAliases: aliases,
			Prefixes:         compactStrings([]string{ontology.Prefix}),
			SourceURL:        derefStr(ontology.SourceUrl),
		},
		Imports: append([]string(nil), version.ImportedOntologies...),
	}
	if len(sourceFiles) > 0 {
		sort.Strings(sourceFiles)
		manifest.Sources = &ontologyVendorSources{Files: sourceFiles}
	}
	payload, err := encodeOntologyVendorManifest(manifest)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "ontology.yaml", payload)
}

// vendoredNamespaceAliases returns every additional namespace URI the prefix
// is globally bound to beyond canonical — the drift variants a version's
// serialization may use (weave_namespace_bindings keeps one row per URI per
// prefix, e.g. crmgeo → ics.forth.gr + dlnarratives + registry namespace).
func (m *Materializer) vendoredNamespaceAliases(ctx context.Context, prefix, canonical string) ([]string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil, nil
	}
	rows, err := m.queries.WeaveListGlobalNamespaceBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespace bindings for vendored ontology aliases: %w", err)
	}
	return namespaceAliasesFromBindings(rows, prefix, canonical), nil
}

// namespaceAliasesFromBindings filters the global binding rows down to the
// distinct non-canonical namespaces bound to prefix, sorted for stable
// manifest output.
func namespaceAliasesFromBindings(rows []sqlcgen.WeaveNamespaceBinding, prefix, canonical string) []string {
	seen := map[string]bool{canonical: true}
	var aliases []string
	for _, row := range rows {
		ns := strings.TrimSpace(row.Namespace)
		if row.Prefix != prefix || ns == "" || seen[ns] {
			continue
		}
		seen[ns] = true
		aliases = append(aliases, ns)
	}
	sort.Strings(aliases)
	return aliases
}

func encodeOntologyVendorManifest(manifest ontologyVendorManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal ontology vendor manifest to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize ontology vendor manifest json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode ontology vendor manifest yaml: %w", err)
	}
	return payload, nil
}

func vendorVersionDir(version, fallback string) string {
	version = strings.TrimSpace(version)
	if version != "" {
		return version
	}
	return strings.TrimSpace(fallback)
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
