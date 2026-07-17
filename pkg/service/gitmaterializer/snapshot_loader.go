package gitmaterializer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProjectManifestFile = projectManifest
type ProjectManifestProject = projectManifestProject
type ProjectManifestInheritance = projectManifestInheritance
type ProjectManifestParent = projectManifestParent
type ProjectManifestOntologies = projectManifestOntologies
type ProjectManifestLinkedOntology = projectManifestLinkedOntology
type ProjectManifestNamespaces = projectManifestNamespaces
type ProjectManifestNamespaceBinding = projectManifestNamespaceBinding
type ProjectManifestManifests = projectManifestManifests

type PletkaModFile = pletkaModManifest
type PletkaModModule = pletkaModModule
type PletkaModRequire = pletkaModRequire
type PletkaProjectRequirement = pletkaProjectRequirement
type PletkaOntologyRequirement = pletkaOntologyRequirement
type PletkaSumEntry = pletkaSumEntry

type AdoptionsIndexManifest = adoptionsIndexManifest
type AdoptionsIndexReceiptEntry = adoptionsIndexReceiptEntry
type AdoptionReceiptManifest = adoptionReceiptManifest
type AdoptionReceiptRecord = adoptionReceiptRecord
type AdoptionReceiptSource = adoptionReceiptSource
type AdoptionReceiptContext = adoptionReceiptContext

type ForksIndexManifest = forksIndexManifest
type ForksIndexReceiptEntry = forksIndexReceiptEntry
type ForkReceiptManifest = forkReceiptManifest
type ForkReceiptRecord = forkReceiptRecord
type ForkReceiptSource = forkReceiptSource

type OntologyVendorManifest = ontologyVendorManifest
type OntologyVendorManifestRoot = ontologyVendorManifestRoot
type OntologyVendorSources = ontologyVendorSources

type SnapshotEntityFile struct {
	Path       string
	EntityType string
	EntityID   string
	FieldID    string
	OwnerType  string
	OwnerID    string
	Payload    []byte
}

type SnapshotEntityTree struct {
	Categories          []SnapshotEntityFile
	Fields              []SnapshotEntityFile
	Models              []SnapshotEntityFile
	Collections         []SnapshotEntityFile
	BaseOverrides       []SnapshotEntityFile
	ModelOverrides      []SnapshotEntityFile
	CollectionOverrides []SnapshotEntityFile
}

type AdoptionManifestSet struct {
	Index    AdoptionsIndexManifest
	Receipts map[string]AdoptionReceiptManifest
}

type ForkManifestSet struct {
	Index    ForksIndexManifest
	Receipts map[string]ForkReceiptManifest
}

type VendoredProjectSnapshot struct {
	Module   string
	Version  string
	Snapshot *ProjectSnapshot
}

type VendoredOntologySnapshot struct {
	Module   string
	Version  string
	Snapshot *OntologySnapshot
}

type VendorSnapshotSet struct {
	Projects   []VendoredProjectSnapshot
	Ontologies []VendoredOntologySnapshot
}

type OntologySnapshot struct {
	RootDir  string
	Manifest OntologyVendorManifest
	Sources  map[string][]byte
}

type ProjectSnapshot struct {
	RootDir   string
	Manifest  ProjectManifestFile
	Mod       *PletkaModFile
	Sum       []PletkaSumEntry
	Adoptions *AdoptionManifestSet
	Forks     *ForkManifestSet
	Entities  SnapshotEntityTree
	Vendor    VendorSnapshotSet
}

func LoadProjectSnapshot(rootDir string) (*ProjectSnapshot, error) {
	return loadProjectSnapshot(rootDir, true)
}

func loadProjectSnapshot(rootDir string, includeVendor bool) (*ProjectSnapshot, error) {
	manifest, err := readYAMLFile[ProjectManifestFile](filepath.Join(rootDir, "project.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load project manifest: %w", err)
	}

	mod, err := readOptionalYAMLFile[PletkaModFile](filepath.Join(rootDir, "pletka.mod"))
	if err != nil {
		return nil, fmt.Errorf("load pletka.mod: %w", err)
	}
	if mod == nil {
		return nil, fmt.Errorf("load pletka.mod: file not found")
	}

	sum, err := readOptionalPletkaSum(filepath.Join(rootDir, "pletka.sum"))
	if err != nil {
		return nil, fmt.Errorf("load pletka.sum: %w", err)
	}

	adoptions, err := loadAdoptionManifestSet(rootDir, manifest.Manifests)
	if err != nil {
		return nil, err
	}
	forks, err := loadForkManifestSet(rootDir, manifest.Manifests)
	if err != nil {
		return nil, err
	}

	entities, err := scanProjectEntities(rootDir)
	if err != nil {
		return nil, err
	}

	snapshot := &ProjectSnapshot{
		RootDir:   rootDir,
		Manifest:  manifest,
		Mod:       mod,
		Sum:       sum,
		Adoptions: adoptions,
		Forks:     forks,
		Entities:  entities,
	}

	if includeVendor {
		vendor, err := loadVendorSnapshots(rootDir)
		if err != nil {
			return nil, err
		}
		snapshot.Vendor = vendor
	}

	if err := validateParentDependencySelectors(snapshot.Manifest, snapshot.Mod); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func validateParentDependencySelectors(manifest ProjectManifestFile, mod *PletkaModFile) error {
	if mod == nil || manifest.Inheritance == nil || len(manifest.Inheritance.Parents) == 0 {
		return nil
	}

	reqByProjectID := make(map[string]PletkaProjectRequirement, len(mod.Require.Projects))
	for _, req := range mod.Require.Projects {
		if strings.TrimSpace(req.Mode) != "parent" {
			continue
		}
		projectID := strings.TrimSpace(req.ProjectID)
		if projectID == "" {
			continue
		}
		reqByProjectID[projectID] = req
	}

	for _, parent := range manifest.Inheritance.Parents {
		projectID := strings.TrimSpace(parent.ProjectID)
		if projectID == "" {
			continue
		}
		req, ok := reqByProjectID[projectID]
		if !ok {
			return fmt.Errorf("load pletka.mod: missing parent dependency for inherited project %s", projectID)
		}
		expectedVersion := parentDependencyVersionDraft
		if strings.TrimSpace(parent.SourceMode) == "release" {
			expectedVersion = strings.TrimSpace(parent.SourceVersion)
		}
		if strings.TrimSpace(expectedVersion) == "" {
			expectedVersion = parentDependencyVersionDraft
		}
		if strings.TrimSpace(req.Version) != expectedVersion {
			return fmt.Errorf("load pletka.mod: parent dependency %s version mismatch: manifest=%s mod=%s", projectID, expectedVersion, strings.TrimSpace(req.Version))
		}
	}

	return nil
}

func loadAdoptionManifestSet(rootDir string, manifests *ProjectManifestManifests) (*AdoptionManifestSet, error) {
	if manifests == nil || strings.TrimSpace(manifests.Adoptions) == "" {
		return nil, nil
	}
	indexPath := filepath.Join(rootDir, filepath.FromSlash(strings.TrimSpace(manifests.Adoptions)))
	index, err := readYAMLFile[AdoptionsIndexManifest](indexPath)
	if err != nil {
		return nil, fmt.Errorf("load adoptions index: %w", err)
	}
	out := &AdoptionManifestSet{
		Index:    index,
		Receipts: make(map[string]AdoptionReceiptManifest, len(index.Receipts)),
	}
	baseDir := filepath.Dir(indexPath)
	for _, entry := range index.Receipts {
		receiptPath := filepath.Join(baseDir, filepath.FromSlash(entry.File))
		receipt, err := readYAMLFile[AdoptionReceiptManifest](receiptPath)
		if err != nil {
			return nil, fmt.Errorf("load adoption receipt %s: %w", entry.File, err)
		}
		if strings.TrimSpace(receipt.Adoption.EntityType) != strings.TrimSpace(entry.EntityType) ||
			strings.TrimSpace(receipt.Adoption.EntityID) != strings.TrimSpace(entry.EntityID) {
			return nil, fmt.Errorf("load adoption receipt %s: receipt entity mismatch", entry.File)
		}
		out.Receipts[filepath.ToSlash(entry.File)] = receipt
	}
	return out, nil
}

func loadForkManifestSet(rootDir string, manifests *ProjectManifestManifests) (*ForkManifestSet, error) {
	if manifests == nil || strings.TrimSpace(manifests.Forks) == "" {
		return nil, nil
	}
	indexPath := filepath.Join(rootDir, filepath.FromSlash(strings.TrimSpace(manifests.Forks)))
	index, err := readYAMLFile[ForksIndexManifest](indexPath)
	if err != nil {
		return nil, fmt.Errorf("load forks index: %w", err)
	}
	out := &ForkManifestSet{
		Index:    index,
		Receipts: make(map[string]ForkReceiptManifest, len(index.Receipts)),
	}
	baseDir := filepath.Dir(indexPath)
	for _, entry := range index.Receipts {
		receiptPath := filepath.Join(baseDir, filepath.FromSlash(entry.File))
		receipt, err := readYAMLFile[ForkReceiptManifest](receiptPath)
		if err != nil {
			return nil, fmt.Errorf("load fork receipt %s: %w", entry.File, err)
		}
		if strings.TrimSpace(receipt.Fork.EntityType) != strings.TrimSpace(entry.EntityType) ||
			strings.TrimSpace(receipt.Fork.EntityID) != strings.TrimSpace(entry.EntityID) {
			return nil, fmt.Errorf("load fork receipt %s: receipt entity mismatch", entry.File)
		}
		out.Receipts[filepath.ToSlash(entry.File)] = receipt
	}
	return out, nil
}

func loadVendorSnapshots(rootDir string) (VendorSnapshotSet, error) {
	var out VendorSnapshotSet

	projectRoots, err := findManifestRoots(filepath.Join(rootDir, "vendor", "projects"), "project.yaml")
	if err != nil {
		return out, fmt.Errorf("scan vendored project snapshots: %w", err)
	}
	for _, entry := range projectRoots {
		snapshot, err := loadProjectSnapshot(entry.root, false)
		if err != nil {
			return out, fmt.Errorf("load vendored project %s: %w", entry.module, err)
		}
		module := entry.module
		if snapshot.Mod != nil && strings.TrimSpace(snapshot.Mod.Module.Path) != "" {
			module = strings.TrimSpace(snapshot.Mod.Module.Path)
		}
		out.Projects = append(out.Projects, VendoredProjectSnapshot{
			Module:   module,
			Version:  vendoredProjectVersionFromDir(rootDir, module, entry.root),
			Snapshot: snapshot,
		})
	}

	ontologyRoots, err := findManifestRoots(filepath.Join(rootDir, "vendor", "ontologies"), "ontology.yaml")
	if err != nil {
		return out, fmt.Errorf("scan vendored ontology snapshots: %w", err)
	}
	for _, entry := range ontologyRoots {
		snapshot, err := loadOntologySnapshot(entry.root)
		if err != nil {
			return out, fmt.Errorf("load vendored ontology %s: %w", entry.module, err)
		}
		version := filepath.Base(entry.root)
		out.Ontologies = append(out.Ontologies, VendoredOntologySnapshot{
			Module:   entry.module,
			Version:  version,
			Snapshot: snapshot,
		})
	}

	return out, nil
}

// vendoredProjectVersionFromDir recovers the pinned version segment for a
// vendored project snapshot directory. It mirrors vendoredProjectDir (see
// vendor_snapshot.go), which appends the pinned version as a trailing path
// segment under vendor/projects/<module>/ for non-draft versions and omits
// it entirely for the draft case. Comparing snapshotRoot against the base
// module directory recovers that trailing segment, or "" when there is
// none (draft, or an unexpected directory shape).
func vendoredProjectVersionFromDir(rootDir, module, snapshotRoot string) string {
	moduleDir := filepath.Join(rootDir, "vendor", "projects", filepath.FromSlash(module))
	rel, err := filepath.Rel(moduleDir, snapshotRoot)
	if err != nil || rel == "." || rel == string(filepath.Separator) || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

func loadOntologySnapshot(rootDir string) (*OntologySnapshot, error) {
	manifest, err := readYAMLFile[OntologyVendorManifest](filepath.Join(rootDir, "ontology.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load ontology manifest: %w", err)
	}
	sources := make(map[string][]byte)
	if manifest.Sources != nil {
		for _, rel := range manifest.Sources.Files {
			if err := validateVendoredSourcePath(rel); err != nil {
				return nil, fmt.Errorf("load ontology source %s: %w", rel, err)
			}
			path := filepath.Join(rootDir, filepath.FromSlash(rel))
			payload, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("load ontology source %s: %w", rel, err)
			}
			sources[filepath.ToSlash(rel)] = payload
		}
	}
	return &OntologySnapshot{
		RootDir:  rootDir,
		Manifest: manifest,
		Sources:  sources,
	}, nil
}

// validateVendoredSourcePath rejects manifest-supplied ontology source
// filenames that are absolute or attempt to escape the ontology's root
// directory via ".." path segments. The ontology.yaml manifest is untrusted
// input materialized from a vendored snapshot on disk, so rel must resolve
// strictly inside the ontology root before it is ever joined into a path.
func validateVendoredSourcePath(rel string) error {
	if filepath.IsAbs(rel) {
		return fmt.Errorf("absolute path not allowed")
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path escapes ontology root")
	}
	return nil
}

type manifestRoot struct {
	root   string
	module string
}

func findManifestRoots(baseDir, manifestFile string) ([]manifestRoot, error) {
	if _, err := os.Stat(baseDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []manifestRoot
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != manifestFile {
			return nil
		}
		root := filepath.Dir(path)
		rel, err := filepath.Rel(baseDir, root)
		if err != nil {
			return err
		}
		module := filepath.ToSlash(rel)
		if manifestFile == "ontology.yaml" {
			module = filepath.ToSlash(filepath.Dir(rel))
		}
		out = append(out, manifestRoot{root: root, module: module})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].module != out[j].module {
			return out[i].module < out[j].module
		}
		return out[i].root < out[j].root
	})
	return out, nil
}

func scanProjectEntities(rootDir string) (SnapshotEntityTree, error) {
	var out SnapshotEntityTree
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Clean(path) == filepath.Join(rootDir, "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		entity, ok := classifySnapshotEntityFile(rel)
		if !ok {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		entity.Payload = payload
		switch entity.EntityType {
		case "category":
			out.Categories = append(out.Categories, entity)
		case "field":
			out.Fields = append(out.Fields, entity)
		case "model":
			out.Models = append(out.Models, entity)
		case "collection":
			out.Collections = append(out.Collections, entity)
		case "base_override":
			out.BaseOverrides = append(out.BaseOverrides, entity)
		case "model_override":
			out.ModelOverrides = append(out.ModelOverrides, entity)
		case "collection_override":
			out.CollectionOverrides = append(out.CollectionOverrides, entity)
		}
		return nil
	})
	if err != nil {
		return SnapshotEntityTree{}, fmt.Errorf("scan project entity files: %w", err)
	}
	sortSnapshotEntityFiles(out.Categories)
	sortSnapshotEntityFiles(out.Fields)
	sortSnapshotEntityFiles(out.Models)
	sortSnapshotEntityFiles(out.Collections)
	sortSnapshotEntityFiles(out.BaseOverrides)
	sortSnapshotEntityFiles(out.ModelOverrides)
	sortSnapshotEntityFiles(out.CollectionOverrides)
	return out, nil
}

func classifySnapshotEntityFile(rel string) (SnapshotEntityFile, bool) {
	rel = filepath.ToSlash(rel)
	segments := strings.Split(rel, "/")
	if len(segments) < 2 {
		return SnapshotEntityFile{}, false
	}

	switch segments[0] {
	case "categories":
		if len(segments) != 2 || !strings.HasSuffix(segments[1], ".yaml") {
			return SnapshotEntityFile{}, false
		}
		return SnapshotEntityFile{
			Path:       rel,
			EntityType: "category",
			EntityID:   strings.TrimSuffix(segments[1], ".yaml"),
		}, true
	case "fields":
		if len(segments) == 3 && segments[2] == "field.yaml" {
			return SnapshotEntityFile{
				Path:       rel,
				EntityType: "field",
				EntityID:   segments[1],
			}, true
		}
		if len(segments) == 3 && segments[2] == "base-override.yaml" {
			return SnapshotEntityFile{
				Path:       rel,
				EntityType: "base_override",
				FieldID:    segments[1],
			}, true
		}
	case "models":
		if len(segments) == 3 && segments[2] == "model.yaml" {
			return SnapshotEntityFile{
				Path:       rel,
				EntityType: "model",
				EntityID:   segments[1],
			}, true
		}
		if len(segments) == 4 && segments[2] == "overrides" && strings.HasSuffix(segments[3], ".yaml") {
			fieldID, entityID, ok := parseOverrideFileName(segments[3])
			if !ok {
				return SnapshotEntityFile{}, false
			}
			return SnapshotEntityFile{
				Path:       rel,
				EntityType: "model_override",
				FieldID:    fieldID,
				EntityID:   entityID,
				OwnerType:  "model",
				OwnerID:    segments[1],
			}, true
		}
	case "collections":
		if len(segments) == 3 && segments[2] == "collection.yaml" {
			return SnapshotEntityFile{
				Path:       rel,
				EntityType: "collection",
				EntityID:   segments[1],
			}, true
		}
		if len(segments) == 4 && segments[2] == "overrides" && strings.HasSuffix(segments[3], ".yaml") {
			fieldID, entityID, ok := parseOverrideFileName(segments[3])
			if !ok {
				return SnapshotEntityFile{}, false
			}
			return SnapshotEntityFile{
				Path:       rel,
				EntityType: "collection_override",
				FieldID:    fieldID,
				EntityID:   entityID,
				OwnerType:  "collection",
				OwnerID:    segments[1],
			}, true
		}
	}

	return SnapshotEntityFile{}, false
}

func parseOverrideFileName(name string) (fieldID string, entityID string, ok bool) {
	if !strings.HasSuffix(name, ".yaml") {
		return "", "", false
	}
	trimmed := strings.TrimSuffix(name, ".yaml")
	parts := strings.SplitN(trimmed, "@", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func sortSnapshotEntityFiles(items []SnapshotEntityFile) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		if items[i].EntityType != items[j].EntityType {
			return items[i].EntityType < items[j].EntityType
		}
		if items[i].OwnerID != items[j].OwnerID {
			return items[i].OwnerID < items[j].OwnerID
		}
		if items[i].FieldID != items[j].FieldID {
			return items[i].FieldID < items[j].FieldID
		}
		return items[i].EntityID < items[j].EntityID
	})
}

func readOptionalPletkaSum(path string) ([]PletkaSumEntry, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parsePletkaSum(payload)
}

func parsePletkaSum(payload []byte) ([]PletkaSumEntry, error) {
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	entries := make([]PletkaSumEntry, 0, len(lines))
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, fmt.Errorf("line %d: expected module and version", idx+1)
		}
		entry := PletkaSumEntry{
			Module:  parts[0],
			Version: parts[1],
		}
		for _, token := range parts[2:] {
			switch {
			case strings.HasPrefix(token, "git:"):
				entry.GitSHA = strings.TrimPrefix(token, "git:")
			case strings.HasPrefix(token, "tree:sha256:"):
				entry.TreeSHA = strings.TrimPrefix(token, "tree:sha256:")
			default:
				return nil, fmt.Errorf("line %d: unsupported token %q", idx+1, token)
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func readOptionalYAMLFile[T any](path string) (*T, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out, err := decodeCanonicalYAML[T](payload)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func readYAMLFile[T any](path string) (T, error) {
	var zero T
	payload, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	out, err := decodeCanonicalYAML[T](payload)
	if err != nil {
		return zero, err
	}
	return out, nil
}

func decodeCanonicalYAML[T any](payload []byte) (T, error) {
	var zero T
	var normalized any
	if err := yaml.Unmarshal(payload, &normalized); err != nil {
		return zero, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}
