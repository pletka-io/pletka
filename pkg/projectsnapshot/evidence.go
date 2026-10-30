package projectsnapshot

import (
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

// Evidence retains a distinct draft, verified or legacy contract. Zero is invalid.
// It proves value consistency, not database provenance or authorization. The
// bound assembler obtains the inputs and remains responsible for that boundary.
type Evidence struct {
	id           Identity
	contract     int
	descriptors  releasedescriptor.Snapshot
	verified     releasecontent.Verified
	legacy       releasecontent.Legacy
	artifact     ontologyevidence.Artifact
	publications []ontologyartifact.ProjectArtifact
	references   []ontologyartifact.TermResolution
}

// VerifiedEvidence binds contract 1 receipts, descriptors and exact publications.
func VerifiedEvidence(receipt releasecontent.Verified, descriptors releasedescriptor.Snapshot, publications []ontologyartifact.ProjectArtifact, references []ontologyartifact.TermResolution) (Evidence, error) {
	id, err := Release(receipt.Origin().ProjectID(), receipt.Origin().Version())
	if err != nil {
		return Evidence{}, err
	}
	e := Evidence{id: id, contract: 1, verified: receipt, descriptors: descriptors}
	return e.withPublications(publications, references)
}

// DraftEvidence retains explicit draft interpretation inputs. No release receipt
// or successful empty-version fallback can create this selection.
func DraftEvidence(id Identity, descriptors releasedescriptor.Snapshot, publications []ontologyartifact.ProjectArtifact, references []ontologyartifact.TermResolution) (Evidence, error) {
	if !id.valid() || !id.draft {
		return Evidence{}, fmt.Errorf("draft evidence requires an explicit draft identity")
	}
	e := Evidence{id: id, descriptors: descriptors}
	return e.withPublications(publications, references)
}

// LegacyEvidence preserves exact contract 2 input and unresolved assessments.
// Neither primary flags nor a successful namespace map promote this contract.
func LegacyEvidence(receipt releasecontent.Legacy, descriptors releasedescriptor.Snapshot, artifact ontologyevidence.Artifact) (Evidence, error) {
	id, err := Release(receipt.Origin().ProjectID(), receipt.Origin().Version())
	if err != nil {
		return Evidence{}, err
	}
	ref, err := artifact.Reference()
	if err != nil {
		return Evidence{}, err
	}
	if ref != receipt.Ontology() {
		return Evidence{}, fmt.Errorf("legacy ontology evidence differs from content receipt")
	}
	e := Evidence{id: id, contract: 2, legacy: receipt, artifact: artifact, descriptors: descriptors}
	if err := e.checkDescriptors(); err != nil {
		return Evidence{}, err
	}
	return e, nil
}

func (e Evidence) checkDescriptors() error {
	if e.descriptors.ProjectID() != e.id.project || e.descriptors.Version() != e.id.version || e.descriptors.Module() == "" {
		return fmt.Errorf("descriptor identity differs from project content")
	}
	return nil
}

func (e Evidence) withPublications(publications []ontologyartifact.ProjectArtifact, references []ontologyartifact.TermResolution) (Evidence, error) {
	if err := e.checkDescriptors(); err != nil {
		return Evidence{}, err
	}
	if publications == nil || references == nil {
		return Evidence{}, fmt.Errorf("ontology inventory and reference coverage must be explicitly loaded")
	}
	inventory, err := newPublicationInventory(publications)
	if err != nil {
		return Evidence{}, err
	}
	if err := inventory.checkReferences(references); err != nil {
		return Evidence{}, err
	}
	e.publications = append([]ontologyartifact.ProjectArtifact{}, publications...)
	e.references = append([]ontologyartifact.TermResolution{}, references...)
	return e, nil
}

type semanticKey struct{ kind, id, location string }

// Identity returns the evidence's immutable owner selection.
func (e Evidence) Identity() Identity { return e.id }

// Contract is 0 for explicit draft evidence, 1 for verified, or 2 for legacy.
// Identity must also be valid; the zero Evidence is never a draft.
func (e Evidence) Contract() int { return e.contract }

// Descriptors returns an immutable actor/module value, not a live lookup.
func (e Evidence) Descriptors() releasedescriptor.Snapshot { return e.descriptors }

// Publications returns detached values only for the explicit published path.
func (e Evidence) Publications() ([]ontologyartifact.ProjectArtifact, error) {
	if !e.id.valid() || e.contract == 2 {
		return nil, fmt.Errorf("published evidence is unavailable")
	}
	return append([]ontologyartifact.ProjectArtifact{}, e.publications...), nil
}

// References returns the exact owned semantic resolutions, without live fallback.
func (e Evidence) References() ([]ontologyartifact.TermResolution, error) {
	if !e.id.valid() || e.contract == 2 {
		return nil, fmt.Errorf("published semantic references are unavailable")
	}
	return append([]ontologyartifact.TermResolution{}, e.references...), nil
}

// Legacy returns exact legacy evidence only, without semantic promotion.
func (e Evidence) Legacy() (ontologyevidence.Artifact, error) {
	if !e.id.valid() || e.contract != 2 {
		return ontologyevidence.Artifact{}, fmt.Errorf("legacy evidence is unavailable")
	}
	return e.artifact, nil
}

func (e Evidence) checkCounts(counts map[string]int64) error {
	switch e.contract {
	case 1:
		return e.verified.CheckCounts(counts)
	case 2:
		return e.legacy.CheckCounts(counts)
	default:
		return nil // Draft counts are supplied separately by the bound loader.
	}
}

type publicationInventory struct {
	docs    map[string]ontologyartifact.Document
	bodies  map[string]ontologyartifact.Body
	digests map[string]string
}

func newPublicationInventory(publications []ontologyartifact.ProjectArtifact) (publicationInventory, error) {
	docs := make(map[string]ontologyartifact.Document, len(publications))
	owners := make(map[string]bool, len(publications))
	digests := make(map[string]string, len(publications))
	bodies := make(map[string]ontologyartifact.Body, len(publications))
	for _, p := range publications {
		d, err := p.Artifact.Decode()
		if err != nil {
			return publicationInventory{}, err
		}
		if _, ok := docs[d.VersionID]; ok || owners[d.OntologyID] {
			return publicationInventory{}, fmt.Errorf("duplicate or conflicting published ontology identity")
		}
		if p.Primary && !p.Linked {
			return publicationInventory{}, fmt.Errorf("primary ontology is not directly linked")
		}
		body, err := ontologyartifact.DecodeBody(d.BodyJSON)
		if err != nil {
			return publicationInventory{}, err
		}
		bodies[d.VersionID] = body
		docs[d.VersionID], owners[d.OntologyID], digests[d.VersionID] = d, true, p.Artifact.Digest
	}
	for _, d := range docs {
		for _, pin := range d.Pins {
			if docs[pin.VersionID].OntologyID != pin.OntologyID || digests[pin.VersionID] != pin.Digest {
				return publicationInventory{}, fmt.Errorf("ontology publication closure is incomplete")
			}
		}
	}
	return publicationInventory{docs: docs, bodies: bodies, digests: digests}, nil
}

func (inventory publicationInventory) checkReferences(references []ontologyartifact.TermResolution) error {
	seen := map[semanticKey]bool{}
	for _, r := range references {
		key := semanticKey{r.EntityKind, r.EntityID, r.Location}
		if seen[key] || key.kind == "" || key.id == "" || key.location == "" {
			return fmt.Errorf("duplicate or invalid semantic location")
		}
		seen[key] = true
		d, ok := inventory.docs[r.VersionID]
		if !ok || d.OntologyID != r.OntologyID || inventory.digests[r.VersionID] != r.Digest {
			return fmt.Errorf("semantic reference has no matching publication")
		}
		body := inventory.bodies[r.VersionID]
		terms := body.Classes
		switch r.Reference.Kind {
		case kindClass:
		case "property":
			terms = body.Properties
		default:
			return fmt.Errorf("semantic reference has invalid term kind")
		}
		if r.URI == "" || !slices.ContainsFunc(terms, func(t ontologyartifact.Term) bool { return t.URI == r.URI }) {
			return fmt.Errorf("semantic reference URI is absent from retained body")
		}
	}
	return nil
}
