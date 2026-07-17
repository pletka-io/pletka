package ontology

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pletka-io/pletka/pkg/domain"
	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

type ImportCompanionFile struct {
	File        string
	Description string
}

type ImportCompanionResult struct {
	File          string
	Description   string
	ClassCount    int
	PropertyCount int
}

type ImportVersionFileOptions struct {
	BaseDir                string
	File                   string
	Ontology               *domain.Ontology
	VersionID              string
	VersionString          string
	CompatibleBaseVersions []string
	SetActive              bool
	NamespaceBindings      []NamespaceBinding
	Companions             []ImportCompanionFile
	SkipMissingCompanions  bool
}

type ImportVersionFileResult struct {
	Input             ImportVersionInput
	NamespaceBindings []NamespaceBinding
	Companions        []ImportCompanionResult
	SkippedCompanions []string
}

func BuildImportVersionInputFromFile(opts ImportVersionFileOptions) (*ImportVersionFileResult, error) {
	if opts.Ontology == nil {
		return nil, fmt.Errorf("ontology is required")
	}
	rdfPath := filepath.Join(opts.BaseDir, opts.File)
	parser := NewImportRDFParser(opts.NamespaceBindings)
	pr, err := parser.ParseFile(rdfPath)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rdfPath, err)
	}

	nsBindings := MergeNamespaceBindings(opts.NamespaceBindings, NamespaceBindingsFromDeclarations(pr.DeclaredNamespaces))
	companionSources, companions, skipped, err := mergeImportCompanions(opts.BaseDir, opts.Companions, pr, opts.SkipMissingCompanions, nsBindings)
	if err != nil {
		return nil, err
	}
	nsBindings = MergeNamespaceBindings(nsBindings, NamespaceBindingsFromDeclarations(pr.DeclaredNamespaces))

	input := BuildImportVersionInput(opts.Ontology, ImportVersionBuildOptions{
		VersionID:              opts.VersionID,
		VersionString:          opts.VersionString,
		CompatibleBaseVersions: opts.CompatibleBaseVersions,
		SetActive:              opts.SetActive,
		CompanionSources:       companionSources,
	}, pr, BuildImportNamespaceResolver(nsBindings))

	return &ImportVersionFileResult{
		Input:             input,
		NamespaceBindings: nsBindings,
		Companions:        companions,
		SkippedCompanions: skipped,
	}, nil
}

func mergeImportCompanions(baseDir string, companions []ImportCompanionFile, pr *parsedontology.ParseResult, skipMissing bool, nsBindings []NamespaceBinding) (map[string]string, []ImportCompanionResult, []string, error) {
	if len(companions) == 0 {
		return nil, nil, nil, nil
	}
	parser := NewImportRDFParser(nsBindings)
	sources := map[string]string{}
	results := make([]ImportCompanionResult, 0, len(companions))
	var skipped []string
	var meta []map[string]any
	for _, companion := range companions {
		companionPath := filepath.Join(baseDir, companion.File)
		if _, err := os.Stat(companionPath); err != nil {
			if skipMissing {
				skipped = append(skipped, companion.File)
				continue
			}
			return nil, nil, nil, fmt.Errorf("companion file %s: %w", companion.File, err)
		}
		cpr, err := parser.ParseFile(companionPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse companion %s: %w", companion.File, err)
		}
		for _, c := range cpr.Classes {
			sources[c.URI] = companion.File
		}
		for _, p := range cpr.Properties {
			sources[p.URI] = companion.File
		}
		pr.Classes = append(pr.Classes, cpr.Classes...)
		pr.Properties = append(pr.Properties, cpr.Properties...)
		pr.DeclaredNamespaces = append(pr.DeclaredNamespaces, cpr.DeclaredNamespaces...)
		pr.MissingNamespaces = append(pr.MissingNamespaces, cpr.MissingNamespaces...)
		result := ImportCompanionResult{
			File:          companion.File,
			Description:   companion.Description,
			ClassCount:    len(cpr.Classes),
			PropertyCount: len(cpr.Properties),
		}
		results = append(results, result)
		meta = append(meta, map[string]any{
			"file":           result.File,
			"description":    result.Description,
			"class_count":    result.ClassCount,
			"property_count": result.PropertyCount,
		})
	}
	if len(meta) > 0 && pr.Version != nil {
		pr.Version.OntologyMetadata = mergeCompanionMetadata(pr.Version.OntologyMetadata, meta)
	}
	return sources, results, skipped, nil
}

// mergeCompanionMetadata folds the companion descriptors into the version's
// existing ontology_metadata JSONB under a "companions" key.
func mergeCompanionMetadata(existing json.RawMessage, companions []map[string]any) json.RawMessage {
	metadataMap := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &metadataMap)
	}
	metadataMap["companions"] = companions
	out, err := json.Marshal(metadataMap)
	if err != nil {
		return existing
	}
	return json.RawMessage(out)
}
