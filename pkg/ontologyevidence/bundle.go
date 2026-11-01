package ontologyevidence

import (
	"fmt"
	"slices"
	"strings"
)

// ValidateBundle validates all artifacts and their shared body/context identities
// without database access. Restore orchestrators must call it during preflight,
// before committing independently restored dependencies or project content.
func ValidateBundle(artifacts []Artifact) error {
	_, _, err := restoreValues(artifacts)
	return err
}

// Bundle contains validated detached restore inputs. Its private inventories
// cannot be changed by a caller or used to acquire database access.
type Bundle struct {
	bodies    []Body
	artifacts []Artifact
}

// PrepareBundle validates every root and shared source before any writes begin.
func PrepareBundle(artifacts []Artifact) (Bundle, error) {
	bodies, evidence, err := restoreValues(artifacts)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{bodies: bodies, artifacts: evidence}, nil
}

// Bodies returns independent body selections in deterministic version order.
func (b Bundle) Bodies() []Body {
	result := slices.Clone(b.bodies)
	for i := range result {
		result[i].Selection = slices.Clone(result[i].Selection)
	}
	return result
}

// Artifacts returns exact original documents in project/release order.
func (b Bundle) Artifacts() []Artifact { return slices.Clone(b.artifacts) }

// Validate every artifact before opening the transaction. Keep inventories
// keyed by concrete identity, not owner or version label; source selections
// remain untouched inside each original artifact. Sort all body locks globally.
func restoreValues(artifacts []Artifact) ([]Body, []Artifact, error) {
	var preparer Preparer
	values := make([]Prepared, 0, len(artifacts))
	for _, a := range artifacts {
		value, err := preparer.Prepare(a)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, value)
	}
	return restorePreparedValues(values)
}

// ValidatePreparedBundle checks cross-artifact body and context consistency
// without repeating the validation already sealed into each capability.
func ValidatePreparedBundle(values []Prepared) error {
	_, _, err := restorePreparedValues(values)
	return err
}

func restorePreparedValues(values []Prepared) ([]Body, []Artifact, error) {
	bodies := map[string]Body{}
	evidence := map[Origin]Artifact{}
	contextValues := map[Origin]string{}
	sourceValues := map[Origin]Source{}
	for _, value := range values {
		a, err := value.Artifact()
		if err != nil {
			return nil, nil, err
		}
		d := value.value.contexts[value.value.root].document
		o := value.value.root
		if old, exists := evidence[o]; exists && old != a {
			return nil, nil, fmt.Errorf("conflicting imported snapshots for %s@%s", d.ProjectID, d.Version)
		}
		evidence[o] = a
		contexts, err := restoreContexts(value, sourceValues, contextValues)
		if err != nil {
			return nil, nil, err
		}
		for _, context := range contexts {
			for _, b := range context.Inventory {
				if old, exists := bodies[b.VersionID]; exists && (old.OntologyID != b.OntologyID || old.Digest != b.Digest || old.BodyJSON != b.BodyJSON) {
					return nil, nil, fmt.Errorf("conflicting imported bodies for %s", b.VersionID)
				}
				bodies[b.VersionID] = b
			}
		}
	}
	orderedBodies := make([]Body, 0, len(bodies))
	for _, b := range bodies {
		orderedBodies = append(orderedBodies, b)
	}
	slices.SortFunc(orderedBodies, func(a, b Body) int { return strings.Compare(a.VersionID, b.VersionID) })
	origins := make([]Origin, 0, len(evidence))
	for o := range evidence {
		origins = append(origins, o)
	}
	slices.SortFunc(origins, func(a, b Origin) int {
		if c := strings.Compare(a.project, b.project); c != 0 {
			return c
		}
		return strings.Compare(a.version, b.version)
	})
	orderedEvidence := make([]Artifact, 0, len(origins))
	for _, o := range origins {
		orderedEvidence = append(orderedEvidence, evidence[o])
	}
	return orderedBodies, orderedEvidence, nil
}

// Collect the root and source contexts, checking their identities against all
// previously collected bundle contexts. This operates only on detached values.
func restoreContexts(value Prepared, sourceValues map[Origin]Source, contextValues map[Origin]string) ([]Document, error) {
	d := value.value.contexts[value.value.root].document
	contexts := []Document{d}
	for _, source := range d.Sources {
		// These identities/digests were checked against this exact flat closure.
		var context Document
		for _, entry := range value.value.contexts {
			if entry.artifact.Digest == source.Digest {
				context = entry.document
				break
			}
		}
		o := Origin{project: context.ProjectID, version: context.Version}
		if old, exists := sourceValues[o]; exists && old != source {
			return nil, fmt.Errorf("conflicting imported source contexts for %s@%s", o.project, o.version)
		}
		sourceValues[o] = source
		contexts = append(contexts, context)
	}
	for origin, context := range value.value.contexts {
		if old, exists := contextValues[origin]; exists && old != context.localDigest {
			return nil, fmt.Errorf("conflicting imported local contexts for %s@%s", origin.project, origin.version)
		}
		contextValues[origin] = context.localDigest
	}
	return contexts, nil
}
