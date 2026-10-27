package gitmaterializer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
)

// This writer receives immutable values only. No database or current policy is
// consulted while serializing descriptor, aliases, sources or the artifact.
func vendorPublishedOntology(rootDir, module string, a ontologyrelease.Artifact, state *vendorState) (*pletkaSumEntry, error) {
	c, err := a.Content()
	if err != nil {
		return nil, err
	}
	if state.ontologies[c.Version.ID] {
		return nil, nil
	}
	versionDir := vendorVersionDir(c.Version.Version, c.Version.ID)
	if versionDir == "." || versionDir == ".." || strings.ContainsAny(versionDir, "/\\") {
		return nil, fmt.Errorf("ontology version is not a safe vendor directory")
	}
	pathKey := module + "@" + versionDir
	if old, ok := state.ontologyPaths[pathKey]; ok && old != c.Version.ID {
		return nil, fmt.Errorf("ontology vendor path collision for %s", pathKey)
	}
	state.ontologyPaths[pathKey] = c.Version.ID
	dir := filepath.Join(rootDir, "vendor", "ontologies", filepath.FromSlash(module), vendorVersionDir(c.Version.Version, c.Version.ID))
	manifest, files, err := publishedOntologyFiles(module, a)
	if err != nil {
		return nil, err
	}
	for name, data := range files {
		if err := writeEntityFile(dir, name, data); err != nil {
			return nil, err
		}
	}
	encoded, err := encodeOntologyVendorManifest(manifest)
	if err != nil {
		return nil, err
	}
	if err := writeEntityFile(dir, "ontology.yaml", encoded); err != nil {
		return nil, err
	}
	hash, err := hashDirectoryTree(dir)
	if err != nil {
		return nil, err
	}
	state.ontologies[c.Version.ID] = true
	return &pletkaSumEntry{Module: module, Version: c.Version.Version, TreeSHA: hash}, nil
}

func publishedOntologyFiles(module string, a ontologyrelease.Artifact) (ontologyVendorManifest, map[string][]byte, error) {
	c, err := a.Content()
	if err != nil {
		return ontologyVendorManifest{}, nil, err
	}
	manifest := ontologyVendorManifest{SchemaVersion: 2, Artifact: &ontologyVendorArtifact{File: ontologyArtifactFile, Digest: a.Digest}, Ontology: ontologyVendorManifestRoot{Module: module, OntologyID: c.Ontology.ID, VersionID: c.Version.ID, Version: c.Version.Version, Slug: ontologyModuleSlug(c.Ontology.Prefix, c.Ontology.Name, c.Ontology.ID), Title: c.Ontology.Name, Kind: c.Ontology.Type, Namespace: c.Ontology.Namespace, Prefixes: compactStrings([]string{c.Ontology.Prefix}), SourceURL: c.Ontology.SourceURL}, Imports: c.Version.Imports, Sources: &ontologyVendorSources{}}
	files := map[string][]byte{ontologyArtifactFile: []byte(a.JSON)}
	add := func(filename, content string) (string, error) {
		if err := validateVendoredSourcePath(filename); err != nil {
			return "", err
		}
		rel := filepath.ToSlash(filepath.Join("src", filename))
		if _, exists := files[rel]; exists {
			return "", fmt.Errorf("duplicate ontology source path %s", rel)
		}
		files[rel] = []byte(content)
		manifest.Sources.Files = append(manifest.Sources.Files, rel)
		return rel, nil
	}
	filename := c.Version.Filename
	if filename == "" {
		filename = c.Ontology.ID + ".ttl"
	}
	if c.Version.RDF == "" {
		return manifest, nil, fmt.Errorf("published ontology %s has no source bytes", c.Version.ID)
	}
	manifest.Sources.Primary, err = add(filename, c.Version.RDF)
	if err != nil {
		return manifest, nil, err
	}
	for _, companion := range c.Companions {
		rel, err := add(companion.Filename, companion.Content)
		if err != nil {
			return manifest, nil, err
		}
		manifest.Sources.Companions = append(manifest.Sources.Companions, ontologyVendorCompanion{File: rel, Description: companion.Description})
	}
	seen := map[string]bool{c.Ontology.Namespace: true}
	for _, b := range c.Bindings {
		if b.Prefix == c.Ontology.Prefix && !seen[b.Namespace] {
			seen[b.Namespace] = true
			manifest.Ontology.NamespaceAliases = append(manifest.Ontology.NamespaceAliases, b.Namespace)
		}
	}
	sort.Strings(manifest.Ontology.NamespaceAliases)
	sort.Strings(manifest.Sources.Files)
	return manifest, files, nil
}

func validatePublishedOntologyFiles(manifest ontologyVendorManifest, sources map[string][]byte, a ontologyrelease.Artifact) error {
	expected, files, err := publishedOntologyFiles(manifest.Ontology.Module, a)
	if err != nil {
		return err
	}
	left, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	right, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(left, right) {
		return fmt.Errorf("ontology vendor manifest differs from its published artifact")
	}
	delete(files, ontologyArtifactFile)
	if len(sources) != len(files) {
		return fmt.Errorf("ontology vendor source inventory differs from artifact")
	}
	for name, content := range files {
		if !bytes.Equal(sources[name], content) {
			return fmt.Errorf("ontology vendor source %s differs from artifact", name)
		}
	}
	return nil
}
