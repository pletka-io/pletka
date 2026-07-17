package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// SemanticID is a parsed representation of a Pletka semantic identifier.
//
// Formats:
//
//	LAF.309           → project=LA,  typeCode=F,   entityType=field,      number=309
//	SRDM.5            → project=SRD, typeCode=M,   entityType=model,      number=5
//	SRDC.8            → project=SRD, typeCode=C,   entityType=collection, number=8
//	LA.CAT.5          → project=LA,  typeCode=CAT, entityType=category,   number=5
//	SEM.CL.6          → project=SEM, typeCode=CL,  entityType=concept_list, number=6
//	LAM.15_person     → project=LA,  typeCode=M,   entityType=model,      number=15, systemName=person
type SemanticID struct {
	Raw        string // original input (e.g., "SRDM.5" or "LAM.15_person")
	ProjectID  string // project prefix (e.g., "SRD", "LA")
	TypeCode   string // short code (e.g., "F", "M", "C", "CAT")
	EntityType string // canonical type name (e.g., "field", "model", "collection", "category")
	Number     int    // numeric suffix (e.g., 5, 309)
	SystemName string // optional system_name from _suffix (e.g., "person")
}

// typeCodeMap maps short codes to entity type names.
var typeCodeMap = map[string]string{
	"F":   "field",
	"M":   "model",
	"C":   "collection",
	"CAT": "category",
	"CL":  "concept_list",
}

// entityTypeToPlural maps entity types to their URL path segment.
var entityTypeToPlural = map[string]string{
	"field":        "fields",
	"model":        "models",
	"collection":   "collections",
	"category":     "categories",
	"concept_list": "concept-lists",
}

// ParseSemanticID parses a semantic identifier string into its components.
// Returns a SemanticID with Valid() == false if the format is unrecognized.
func ParseSemanticID(raw string) SemanticID {
	sid := SemanticID{Raw: raw}

	if raw == "" {
		return sid
	}

	// Split off optional system_name after underscore.
	idPart := raw
	if idx := strings.Index(raw, "_"); idx > 0 {
		idPart = raw[:idx]
		sid.SystemName = raw[idx+1:]
	}

	parts := strings.Split(idPart, ".")

	// Namespaced pattern: PREFIX.CAT.NUMBER, PREFIX.CL.NUMBER (3 parts)
	if len(parts) == 3 && (parts[1] == "CAT" || parts[1] == "CL") {
		sid.ProjectID = parts[0]
		sid.TypeCode = parts[1]
		sid.EntityType = typeCodeMap[parts[1]]
		sid.Number, _ = strconv.Atoi(parts[2])
		return sid
	}

	// Standard pattern: PREFIXTYPE.NUMBER (2 parts)
	if len(parts) == 2 {
		prefix := parts[0]
		sid.Number, _ = strconv.Atoi(parts[1])

		for _, code := range []string{"F", "M", "C"} {
			if strings.HasSuffix(prefix, code) {
				sid.ProjectID = strings.TrimSuffix(prefix, code)
				sid.TypeCode = code
				sid.EntityType = typeCodeMap[code]
				return sid
			}
		}

		// Unknown type code — store prefix as-is.
		sid.ProjectID = prefix
	}

	return sid
}

// Valid reports whether the semantic ID was successfully parsed into a
// recognized entity type with a project prefix and number.
func (s SemanticID) Valid() bool {
	return s.ProjectID != "" && s.EntityType != "" && s.Number > 0
}

// ID returns the canonical semantic ID without system_name suffix.
// e.g., for "LAM.15_person" it returns "LAM.15".
func (s SemanticID) ID() string {
	if s.TypeCode == "CAT" || s.TypeCode == "CL" {
		return fmt.Sprintf("%s.%s.%d", s.ProjectID, s.TypeCode, s.Number)
	}
	if s.TypeCode != "" && s.ProjectID != "" {
		return fmt.Sprintf("%s%s.%d", s.ProjectID, s.TypeCode, s.Number)
	}
	return s.Raw
}

// URL returns the absolute URL path for this entity.
// e.g., "SRDM.5" → "/projects/SRD/models/SRDM.5"
func (s SemanticID) URL() string {
	plural, ok := entityTypeToPlural[s.EntityType]
	if !ok || s.ProjectID == "" {
		return ""
	}
	return fmt.Sprintf("/projects/%s/%s/%s", s.ProjectID, plural, s.ID())
}

// refTypeToPlural maps the override ref-type discriminator
// ("resource_model", "collection_model") used in weave_override_refs
// to the URL path segment. Used by EntityURLForRefType when a
// SemanticID can't be parsed but the ref-type + projectID + targetID
// are known from context.
var refTypeToPlural = map[string]string{
	"resource_model":   "models",
	"collection_model": "collections",
	"concept_list":     "concept-lists",
}

// EntityURLForRefType returns /projects/{projectID}/{plural}/{id} for
// a known override ref-type. Use as a fallback when SemanticID
// parsing fails (legacy/malformed ref rows) so the API always emits
// a non-empty URL — the frontend should never need a string-
// construction fallback (api-patterns.md).
//
// Returns "" only when the ref-type is unknown or required inputs are
// missing; the caller can decide whether to omit the URL or surface
// the bad row.
func EntityURLForRefType(projectID, refType, targetID string) string {
	plural, ok := refTypeToPlural[refType]
	if !ok || projectID == "" || targetID == "" {
		return ""
	}
	return fmt.Sprintf("/projects/%s/%s/%s", projectID, plural, targetID)
}

// BelongsTo reports whether this entity belongs to the given project.
func (s SemanticID) BelongsTo(projectID string) bool {
	return s.ProjectID == projectID
}

// String returns the raw input string.
func (s SemanticID) String() string {
	return s.Raw
}

// WithSystemName returns a display string in the format "semanticID_systemName".
// If systemName is empty, returns just the semantic ID.
func (s SemanticID) WithSystemName(systemName string) string {
	if systemName == "" {
		return s.ID()
	}
	return s.ID() + "_" + systemName
}
