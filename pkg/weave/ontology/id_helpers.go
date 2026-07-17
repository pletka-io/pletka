package ontology

import "github.com/pletka-io/pletka/pkg/ids"

// Deterministic ID generation for ontology entities. Stable seeds mean
// the same entity gets the same ID across DB resets, which lets the
// importer round-trip via TOML manifests and the planned git
// materializer round-trip via filesystem layouts.

// GenerateFamilyID generates an ID for an ontology family.
// Seed: "family:{slug}".
func GenerateFamilyID(slug string) string {
	if slug == "" {
		return ""
	}
	return ids.GenerateID("family:" + slug)
}

// GenerateOntologyID generates an ID for an ontology.
// Seed: "ontology:{prefix}".
func GenerateOntologyID(prefix string) string {
	if prefix == "" {
		return ""
	}
	return ids.GenerateID("ontology:" + prefix)
}

// GenerateVersionID generates an ID for an ontology version.
// Seed: "version:{prefix}:{version}".
func GenerateVersionID(prefix, version string) string {
	if prefix == "" || version == "" {
		return ""
	}
	return ids.GenerateID("version:" + prefix + ":" + version)
}

// GenerateClassID generates an ID for a class scoped to an ontology version.
// Seed: "class:{versionID}:{uri}".
//
// versionID is part of the seed because extension ontologies may
// redeclare parent classes with different hierarchy positions (e.g.
// CRMarchaeo redeclares CRM's E39_Actor with extra superclasses). Each
// version owns its own row.
func GenerateClassID(versionID, uri string) string {
	if uri == "" {
		return ""
	}
	if versionID == "" {
		return ids.GenerateID("class:" + uri)
	}
	return ids.GenerateID("class:" + versionID + ":" + uri)
}

// GeneratePropertyID generates an ID for a property scoped to an ontology version.
// Seed: "property:{versionID}:{uri}". Same rationale as GenerateClassID.
func GeneratePropertyID(versionID, uri string) string {
	if uri == "" {
		return ""
	}
	if versionID == "" {
		return ids.GenerateID("property:" + uri)
	}
	return ids.GenerateID("property:" + versionID + ":" + uri)
}
