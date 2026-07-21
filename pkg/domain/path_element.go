package domain

// PathElement represents one step in an ontology path (class, property, or literal).
// This is a core domain type used by Field, PathBuilder UI, and path queries.
type PathElement struct {
	Type              string `json:"type"`                          // "class", "property", or "literal"
	URI               string `json:"uri"`                           // Full prefixed URI (e.g., "crm:E21_Person")
	Prefix            string `json:"prefix"`                        // Namespace prefix (e.g., "crm")
	LocalName         string `json:"local_name"`                    // Local name (e.g., "E21_Person")
	Datatype          string `json:"datatype,omitempty"`            // For literals: "rdf:literal", "xsd:date"
	Position          int    `json:"position"`                      // 0-indexed position in path
	Complete          bool   `json:"complete,omitempty"`            // Editor-only: modeler marked the path finished in the path editor. Ignored by resolution, generators, and the path audit.
	ClassCode         string `json:"class_code,omitempty"`          // Short class code (e.g., "E33_E41")
	InstanceID        string `json:"instance_id,omitempty"`         // Legacy hand-coded bracket ID (e.g., "HERF.200_1"); persisted in path_elements JSONB
	PathNode          string `json:"path_node,omitempty"`           // Generated structural ancestor chain (slugified qnames). Set on class elements while building generator snapshots; never persisted.
	PathNodeID        string `json:"path_node_id,omitempty"`        // Short generated node id, {class_code}_{occurrence}, derived from PathNode. The X3ML variable / UI id. Supersedes InstanceID.
	OntologyVersionID string `json:"ontology_version_id,omitempty"` // FK to ontology version for compatibility checks

	// AdditionalTypes holds secondary class instantiations for multi-typed nodes.
	// Example: E29_Design_or_Procedure/crmdig:D1_Digital_Object means the node
	// is simultaneously an instance of both classes. The primary type is in the
	// fields above; additional types are here.
	AdditionalTypes []TypeRef `json:"additional_types,omitempty"`

	// SubPropertyOf is set for CRM ".1" property-class qualifiers.
	// Example: P14.1_in_the_role_of qualifies P14_carried_out_by with a type.
	// The value is the parent property code (e.g., "P14" for "P14.1").
	SubPropertyOf string `json:"sub_property_of,omitempty"`
}

// TypeRef identifies an ontology class used as an additional type on a path element.
type TypeRef struct {
	URI       string `json:"uri"`                     // Full prefixed URI (e.g., "crmdig:D1_Digital_Object")
	Prefix    string `json:"prefix"`                  // Namespace prefix (e.g., "crmdig")
	LocalName string `json:"local_name"`              // Local name (e.g., "D1_Digital_Object")
	ClassCode string `json:"class_code,omitempty"`    // Short class code (e.g., "D1")
}

// PrefixedName returns the namespaced form: "crm:E21_Person".
// For multi-typed elements, includes additional types separated by "/":
// "crm:E29_Design_or_Procedure/crmdig:D1_Digital_Object"
func (pe PathElement) PrefixedName() string {
	var primary string
	if pe.Prefix == "" {
		primary = pe.LocalName
	} else {
		primary = pe.Prefix + ":" + pe.LocalName
	}

	if len(pe.AdditionalTypes) == 0 {
		return primary
	}

	result := primary
	for _, t := range pe.AdditionalTypes {
		result += "/" + t.PrefixedName()
	}
	return result
}

// ShortCode returns the compact class identifier for UI display: the
// class code when present (e.g. "E21"), the derived alpha+digit prefix
// of the local name as a fallback (e.g. "ZE37_Population" → "ZE37"),
// and the raw local name only as a last resort.
//
// The derived fallback covers historical rows whose class_code was
// dropped before parseOntologyScopeInput preserved the field — the
// LeadBox in entity lists keys off this value, so we want
// it always to come back as the short code, not the verbose local name.
func (pe PathElement) ShortCode() string {
	if pe.ClassCode != "" {
		return pe.ClassCode
	}
	if derived := DeriveClassCode(pe.LocalName); derived != "" {
		return derived
	}
	return pe.LocalName
}

// DeriveClassCode extracts the alpha+digits prefix of a CRM-style local
// name. For "E33_E41_Linguistic_Appellation" returns "E33". For
// "P1_is_identified_by" returns "P1". Returns "" when no match.
//
// Pure string operation, no regex needed: walk while a-z + digits.
func DeriveClassCode(localName string) string {
	if localName == "" {
		return ""
	}
	end := 0
	for end < len(localName) {
		c := localName[end]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return ""
	}
	letters := localName[:end]
	digitStart := end
	for end < len(localName) {
		c := localName[end]
		if c >= '0' && c <= '9' {
			end++
			continue
		}
		break
	}
	if end == digitStart {
		return ""
	}
	return letters + localName[digitStart:end]
}

// PrefixedName returns the namespaced form for a TypeRef.
func (t TypeRef) PrefixedName() string {
	if t.Prefix == "" {
		return t.LocalName
	}
	return t.Prefix + ":" + t.LocalName
}

// AllTypes returns all class URIs for this element (primary + additional).
// Useful for RDF generation and ontology validation.
func (pe PathElement) AllTypes() []TypeRef {
	primary := TypeRef{
		URI:       pe.URI,
		Prefix:    pe.Prefix,
		LocalName: pe.LocalName,
		ClassCode: pe.ClassCode,
	}
	if len(pe.AdditionalTypes) == 0 {
		return []TypeRef{primary}
	}
	all := make([]TypeRef, 0, 1+len(pe.AdditionalTypes))
	all = append(all, primary)
	all = append(all, pe.AdditionalTypes...)
	return all
}
