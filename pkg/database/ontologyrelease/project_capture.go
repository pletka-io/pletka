package ontologyrelease

import (
	"context"
	"sort"

	"github.com/pletka-io/pletka/pkg/database/projectontology"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// CaptureProject freezes linked/explicit ontology selections, resolves all
// project semantic references and retains captured source closures. Call after project dependency capture,
// inside the same release transaction. Historical inventories are never inferred.
func CaptureProject(ctx context.Context, q *sqlcgen.Queries, project, version string) error {
	refs, err := q.WeaveProjectSemanticReferences(ctx, sqlcgen.WeaveProjectSemanticReferencesParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	selected, err := q.WeaveProjectOntologySelections(ctx, sqlcgen.WeaveProjectOntologySelectionsParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	sources, err := q.WeaveProjectOntologySourceInventories(ctx, sqlcgen.WeaveProjectOntologySourceInventoriesParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	for _, source := range sources {
		if source.OntologyDependenciesVersion != 1 || !source.SemanticVerified {
			return invalid("source project %s@%s has no verified ontology inventory", source.SourceProjectID, source.SourceVersion)
		}
	}
	inherited, err := q.WeaveProjectOntologySourcePins(ctx, sqlcgen.WeaveProjectOntologySourcePinsParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	pins := make([]Pin, 0, len(inherited))
	for _, pin := range inherited {
		pins = append(pins, Pin{OntologyID: pin.OntologyID, VersionID: pin.OntologyVersionID, Digest: pin.Digest})
	}
	artifacts, err := projectontology.Plan(ctx, projectPlanner{queries: q}, selected, pins, len(refs) > 0)
	if err != nil {
		return err
	}
	p := publisher{pins: map[string]Pin{}, sources: map[string]bodyDocument{}}
	for _, a := range artifacts {
		d, err := a.Decode()
		if err != nil {
			return err
		}
		body, err := decodeBody(d.BodyJSON)
		if err != nil {
			return err
		}
		p.pins[d.OntologyID] = Pin{OntologyID: d.OntologyID, VersionID: d.VersionID, Digest: a.Digest}
		p.sources[d.OntologyID] = body
	}
	owners := make([]string, 0, len(p.pins))
	for owner := range p.pins {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		pin := p.pins[owner]
		if err := q.WeaveInsertProjectOntologyPin(ctx, sqlcgen.WeaveInsertProjectOntologyPinParams{ProjectID: project, Version: version, OntologyID: pin.OntologyID, OntologyVersionID: pin.VersionID, Digest: pin.Digest}); err != nil {
			return err
		}
	}
	if err := q.WeaveMarkProjectOntologyInventory(ctx, sqlcgen.WeaveMarkProjectOntologyInventoryParams{ProjectID: project, Version: version}); err != nil {
		return err
	}
	return captureProjectReferences(ctx, q, &p, project, version, refs)
}
