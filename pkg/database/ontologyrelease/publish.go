package ontologyrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Options makes exceptional choices explicit at publication. Pins map owner IDs
// to stable version IDs, never labels. External lists classify exact source IRIs
// or term references; they are evidence, not an implicit fetch/fallback policy.
type Options struct {
	Pins               map[string]string `json:"pins,omitempty" yaml:"pins,omitempty"`
	ExternalImports    []string          `json:"external_imports,omitempty" yaml:"external_imports,omitempty"`
	ExternalReferences []string          `json:"external_references,omitempty" yaml:"external_references,omitempty"`
}

type publicationQueries interface {
	WeaveOntologyPublicationBody(context.Context, sqlcgen.WeaveOntologyPublicationBodyParams) (sqlcgen.WeaveOntologyPublicationBodyRow, error)
	WeaveOntologyPublicationCandidates(context.Context) ([]sqlcgen.WeaveOntologyPublicationCandidatesRow, error)
	WeaveOntologyPublicationSource(context.Context, sqlcgen.WeaveOntologyPublicationSourceParams) (sqlcgen.WeaveOntologyPublicationSourceRow, error)
	WeavePublishOntology(context.Context, sqlcgen.WeavePublishOntologyParams) error
}

// Capture resolves and persists a publication using the importer's transaction,
// after ontologybody.Seal. The caller MUST own a repeatable-read transaction and
// roll back on any error. No active/latest lookup occurs in released readers.
func Capture(ctx context.Context, q publicationQueries, ref ontologybody.Reference, opts Options) error {
	row, err := q.WeaveOntologyPublicationBody(ctx, sqlcgen.WeaveOntologyPublicationBodyParams{OntologyID: string(ref.OntologyID()), VersionID: string(ref.VersionID())})
	if err != nil {
		return err
	}
	b, err := decodeBody(row.BodyJson)
	if err != nil {
		return err
	}
	candidates, err := q.WeaveOntologyPublicationCandidates(ctx)
	if err != nil {
		return err
	}
	p := publisher{ctx: ctx, q: q, opts: opts, body: b, candidates: candidates, pins: map[string]Pin{}, sources: map[string]bodyDocument{}}
	if err := p.direct(); err != nil {
		return err
	}
	if err := p.references(); err != nil {
		return err
	}
	doc := Document{FormatVersion: 1, OntologyID: b.Ontology.ID, VersionID: b.Version.ID, BodyJSON: row.BodyJson, BodyDigest: row.Digest, Pins: []Pin{}, Requirements: p.requirements}
	for _, pin := range p.pins {
		doc.Pins = append(doc.Pins, pin)
	}
	sort.Slice(doc.Pins, func(i, j int) bool { return doc.Pins[i].OntologyID < doc.Pins[j].OntologyID })
	sort.Slice(doc.Requirements, func(i, j int) bool {
		a, b := doc.Requirements[i], doc.Requirements[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if _, err := (Artifact{JSON: string(data), Digest: digest(string(data))}).Decode(); err != nil {
		return err
	}
	return q.WeavePublishOntology(ctx, sqlcgen.WeavePublishOntologyParams{OntologyID: doc.OntologyID, VersionID: doc.VersionID, Artifact: string(data)})
}

const defaultsPrefix = "defaults"

type publisher struct {
	ctx          context.Context
	q            publicationQueries
	opts         Options
	body         bodyDocument
	candidates   []sqlcgen.WeaveOntologyPublicationCandidatesRow
	requirements []Requirement
	pins         map[string]Pin
	sources      map[string]bodyDocument
}

func (p *publisher) choose(owner, version string) (Pin, error) {
	if explicit, ok := p.opts.Pins[owner]; ok {
		if explicit == "" || (version != "" && explicit != version) {
			return Pin{}, invalid("conflicting or empty explicit pin for ontology %s", owner)
		}
		version = explicit
	}
	for _, c := range p.candidates {
		if c.OntologyID != owner || c.VersionID == nil {
			continue
		}
		if (version != "" && *c.VersionID == version) || (version == "" && c.IsActive) {
			if c.PublishedVersionID == nil || c.Digest == nil {
				return Pin{}, invalid("ontology %s version %s must be published before it can be pinned", owner, *c.VersionID)
			}
			return Pin{OntologyID: owner, VersionID: *c.VersionID, Digest: *c.Digest}, nil
		}
	}
	return Pin{}, invalid("ontology %s has no selected published version; provide a concrete pin", owner)
}

func (p *publisher) add(pin Pin) error {
	if pin.OntologyID == p.body.Ontology.ID {
		return invalid("cyclic ontology dependency on %s", pin.OntologyID)
	}
	if old, ok := p.pins[pin.OntologyID]; ok {
		if old != pin {
			return invalid("conflicting ontology dependency versions for %s: %s and %s", pin.OntologyID, old.VersionID, pin.VersionID)
		}
		return nil
	}
	row, err := p.q.WeaveOntologyPublicationSource(p.ctx, sqlcgen.WeaveOntologyPublicationSourceParams{OntologyID: pin.OntologyID, VersionID: pin.VersionID})
	if err != nil {
		return err
	}
	if row.Digest != pin.Digest {
		return fmt.Errorf("ontology dependency digest mismatch")
	}
	doc, err := (Artifact{JSON: row.Artifact, Digest: row.Digest}).Decode()
	if err != nil {
		return err
	}
	b, err := decodeBody(doc.BodyJSON)
	if err != nil {
		return err
	}
	p.pins[pin.OntologyID] = pin
	p.sources[pin.OntologyID] = b
	for _, dep := range doc.Pins {
		if err := p.add(dep); err != nil {
			return err
		}
	}
	return nil
}

func (p *publisher) direct() error {
	// Resolve ALL direct choices before merging any source's closure. Inherited
	// pins cannot accidentally select a different source's active dependency.
	selected := []Pin{}
	require := func(kind, name, owner, version string) error {
		pin, err := p.choose(owner, version)
		if err != nil {
			return err
		}
		selected = append(selected, pin)
		p.requirements = append(p.requirements, Requirement{Kind: kind, Name: name, Disposition: "pinned", OntologyID: owner})
		return nil
	}
	if p.body.Ontology.Extends != "" {
		if err := require("base", p.body.Ontology.Extends, p.body.Ontology.Extends, ""); err != nil {
			return err
		}
	} else if len(p.body.Version.Compatible) > 0 {
		return invalid("compatible base versions require an extends ontology")
	}
	if err := p.declareDefaults(require); err != nil {
		return err
	}
	if err := p.declareImports(require); err != nil {
		return err
	}
	owners := make([]string, 0, len(p.opts.Pins))
	for owner := range p.opts.Pins {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		if err := require("explicit", owner, owner, p.opts.Pins[owner]); err != nil {
			return err
		}
	}
	for _, pin := range selected {
		if err := p.add(pin); err != nil {
			return err
		}
	}
	if base := p.body.Ontology.Extends; base != "" && len(p.body.Version.Compatible) > 0 && !slices.Contains(p.body.Version.Compatible, p.sources[base].Version.VersionString) {
		return invalid("selected base %s version %s is incompatible", base, p.sources[base].Version.VersionString)
	}
	return nil
}

type requirementSelector func(kind, name, owner, version string) error

func (p *publisher) declareDefaults(require requirementSelector) error {
	defaults := ""
	for _, c := range p.candidates {
		if c.Prefix == defaultsPrefix {
			defaults = c.OntologyID
		}
	}
	switch defaults {
	case "":
		p.requirements = append(p.requirements, Requirement{Kind: defaultsPrefix, Name: defaultsPrefix, Disposition: "absent"})
	case p.body.Ontology.ID:
		p.requirements = append(p.requirements, Requirement{Kind: defaultsPrefix, Name: defaultsPrefix, Disposition: "self"})
	default:
		if err := require(defaultsPrefix, defaultsPrefix, defaults, ""); err != nil {
			return err
		}
	}
	return nil
}

func (p *publisher) declareImports(require requirementSelector) error {
	for _, iri := range unique(p.body.Version.Imports) {
		if slices.Contains(p.opts.ExternalImports, iri) {
			p.requirements = append(p.requirements, Requirement{Kind: "import", Name: iri, Disposition: "external"})
			continue
		}
		owner, version, err := p.importTarget(iri)
		if err != nil {
			return err
		}
		if err := require("import", iri, owner, version); err != nil {
			return err
		}
	}
	for _, iri := range p.opts.ExternalImports {
		if !slices.Contains(p.body.Version.Imports, iri) {
			return invalid("external import %s is not declared by the body", iri)
		}
	}
	return nil
}

func (p *publisher) importTarget(iri string) (string, string, error) {
	exact := map[string]Pin{}
	owners := map[string]bool{}
	for _, c := range p.candidates {
		if c.VersionIri != nil && *c.VersionIri == iri && c.VersionID != nil {
			exact[*c.VersionID] = Pin{OntologyID: c.OntologyID, VersionID: *c.VersionID}
		}
		if c.Namespace == iri || (c.OntologyUri != nil && *c.OntologyUri == iri) {
			owners[c.OntologyID] = true
		}
	}
	if len(exact) == 1 {
		for _, pin := range exact {
			return pin.OntologyID, pin.VersionID, nil
		}
	}
	if len(exact) > 1 || len(owners) != 1 {
		return "", "", invalid("import %s is unresolved or ambiguous; declare an exact external import or disambiguate its ontology metadata", iri)
	}
	for owner := range owners {
		return owner, "", nil
	}
	panic("unreachable")
}

func unique(in []string) []string {
	out := slices.Clone(in)
	sort.Strings(out)
	return slices.Compact(out)
}
