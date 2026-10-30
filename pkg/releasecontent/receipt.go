// Package releasecontent contains detached owned-content capture receipts.
// These are storage evidence, not complete assembled graphs or portable output.
package releasecontent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

// Section includes successfully captured empty sections. SHA256 hashes
// PostgreSQL row values and must not be compared to a serializer's output hash.
type Section struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

type receipt struct {
	origin   ontologyevidence.Origin
	sections map[string]Section
}

// Origin returns the project/release binding supplied by the capture reader.
func (r receipt) Origin() ontologyevidence.Origin { return r.origin }

// Section distinguishes a captured empty section from an absent section.
func (r receipt) Section(name string) (Section, bool) {
	s, ok := r.sections[name]
	return s, ok
}

// CheckCounts requires an explicit loaded count for every owned section. It
// checks row coverage only; it does not prove field, graph or semantic coverage.
func (r receipt) CheckCounts(counts map[string]int64) error {
	if r.origin.ProjectID() == "" || len(counts) != len(r.sections) {
		return fmt.Errorf("loaded content does not cover the captured sections")
	}
	for _, name := range RequiredSections() {
		count, ok := counts[name]
		if !ok || count < 0 || count != r.sections[name].Rows {
			return fmt.Errorf("loaded content count differs for %s", name)
		}
	}
	return nil
}

// Verified is contract 1 storage evidence. Only a bound capture reader establishes
// its provenance; parsing alone does not grant release access or certify output.
type Verified struct{ receipt }

// Legacy is contract 2 structural evidence, with a separately retained ontology
// snapshot. It cannot be used as Verified and does not certify semantic resolution.
type Legacy struct {
	receipt
	ontology ontologyevidence.Reference
}

// Ontology returns the exact captured legacy document binding.
func (r Legacy) Ontology() ontologyevidence.Reference { return r.ontology }

// DecodeVerified validates the exact section inventory and the contract 1 shape.
func DecodeVerified(data []byte, project, version string) (Verified, error) {
	r, d, err := decode(data, project, version)
	if err != nil {
		return Verified{}, err
	}
	if d.Format != 1 || d.Evidence != "" || d.Digest != "" {
		return Verified{}, fmt.Errorf("unsupported verified content receipt")
	}
	return Verified{receipt: r}, nil
}

// DecodeLegacy validates contract 2 without promoting it to verified evidence.
func DecodeLegacy(data []byte, project, version string) (Legacy, error) {
	r, d, err := decode(data, project, version)
	if err != nil {
		return Legacy{}, err
	}
	if d.Format != 2 || d.Evidence != "legacy_ontology_snapshot" {
		return Legacy{}, fmt.Errorf("unsupported legacy content receipt")
	}
	ref, err := ontologyevidence.NewReference(project, version, d.Digest)
	if err != nil {
		return Legacy{}, err
	}
	return Legacy{receipt: r, ontology: ref}, nil
}

type document struct {
	Format   int                    `json:"format_version"`
	Evidence string                 `json:"evidence_kind"`
	Digest   string                 `json:"ontology_snapshot_digest"`
	Sections map[string]wireSection `json:"sections"`
}

type wireSection struct {
	Rows   *int64  `json:"rows"`
	SHA256 *string `json:"sha256"`
}

func decode(data []byte, project, version string) (receipt, document, error) {
	var d document
	origin, err := ontologyevidence.NewOrigin(project, version)
	if err != nil {
		return receipt{}, d, err
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return receipt{}, d, fmt.Errorf("decode content receipt: %w", err)
	}
	names := RequiredSections()
	if len(d.Sections) != len(names) {
		return receipt{}, d, fmt.Errorf("incomplete content receipt inventory")
	}
	sections := make(map[string]Section, len(names))
	for _, name := range names {
		s, ok := d.Sections[name]
		if !ok || s.Rows == nil || *s.Rows < 0 || s.SHA256 == nil || !validDigest(*s.SHA256) {
			return receipt{}, d, fmt.Errorf("missing or invalid content section %s", name)
		}
		sections[name] = Section{Rows: *s.Rows, SHA256: *s.SHA256}
	}
	return receipt{origin: origin, sections: sections}, d, nil
}

func validDigest(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}

// RequiredSections returns the fixed contract 1/2 inventory in stable order.
// Conformance compares it with the reviewed table/column capture ledger.
func RequiredSections() []string {
	return []string{
		"weave_adoptions_archive", "weave_categories_archive",
		"weave_collection_placements_archive", "weave_collections_archive",
		"weave_concept_broader_archive", "weave_concept_list_entries_archive",
		"weave_concept_lists_archive", "weave_entity_forks_archive",
		"weave_example_values_archive", "weave_examples_archive",
		"weave_field_overrides_archive", "weave_fields_archive", "weave_models_archive",
		"weave_override_refs_archive", "weave_project_actors_archive",
		"weave_project_attributions_archive", "weave_project_inheritance_archive",
		"weave_project_ontology_versions_archive", "weave_projects_archive",
		"weave_release_namespace_bindings_archive", "weave_vocabularies_archive",
		"weave_vocabulary_entries_archive",
	}
}
