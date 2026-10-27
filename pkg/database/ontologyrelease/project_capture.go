package ontologyrelease

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// CaptureProject freezes linked/explicit ontology selections and the ontology
// closures of captured source projects. Call after project dependency capture,
// inside the same release transaction. Historical inventories are never inferred.
func CaptureProject(ctx context.Context, q *sqlcgen.Queries, project, version string) error {
	selected, err := q.WeaveProjectOntologySelections(ctx, sqlcgen.WeaveProjectOntologySelectionsParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	sources, err := q.WeaveProjectOntologySourceInventories(ctx, sqlcgen.WeaveProjectOntologySourceInventoriesParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	for _, source := range sources {
		if source.OntologyDependenciesVersion != 1 {
			return invalid("source project %s@%s has no verified ontology inventory", source.SourceProjectID, source.SourceVersion)
		}
	}
	inherited, err := q.WeaveProjectOntologySourcePins(ctx, sqlcgen.WeaveProjectOntologySourcePinsParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	candidates, err := q.WeaveOntologyPublicationCandidates(ctx)
	if err != nil {
		return err
	}
	p := publisher{ctx: ctx, q: q, candidates: candidates, pins: map[string]Pin{}, sources: map[string]bodyDocument{}, opts: Options{Pins: map[string]string{}}}
	direct, err := projectDirectPins(ctx, q, &p, selected)
	if err != nil {
		return err
	}
	// A declared defaults link overrides active policy. Resolve it before any
	// source closure can influence the choice; conflicts must be explicit.
	if len(selected) > 0 {
		for _, c := range candidates {
			if c.Prefix == defaultsPrefix {
				pin, err := p.choose(c.OntologyID, "")
				if err != nil {
					return err
				}
				direct = append(direct, pin)
				break
			}
		}
	}
	for _, pin := range direct {
		if err := p.add(pin); err != nil {
			return err
		}
	}
	for _, pin := range inherited {
		if err := p.add(Pin{OntologyID: pin.OntologyID, VersionID: pin.OntologyVersionID, Digest: pin.Digest}); err != nil {
			return err
		}
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
	return q.WeaveMarkProjectOntologyInventory(ctx, sqlcgen.WeaveMarkProjectOntologyInventoryParams{ProjectID: project, Version: version})
}

func projectDirectPins(ctx context.Context, q *sqlcgen.Queries, p *publisher, selected []string) ([]Pin, error) {
	direct := []Pin{}
	for _, id := range selected {
		row, err := q.WeavePublishedOntologyIdentity(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalid("ontology version %s is not published; publish it before cutting a project release", id)
		}
		if err != nil {
			return nil, err
		}
		if old, ok := p.opts.Pins[row.OntologyID]; ok && old != row.VersionID {
			return nil, invalid("project selects conflicting ontology versions for %s", row.OntologyID)
		}
		p.opts.Pins[row.OntologyID] = row.VersionID
		direct = append(direct, Pin{OntologyID: row.OntologyID, VersionID: row.VersionID, Digest: row.Digest})
	}
	return direct, nil
}
