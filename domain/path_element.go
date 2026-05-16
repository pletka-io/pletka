package domain

// PathElement represents one step in an ontology path.
type PathElement struct {
	Type              string    `json:"type"`
	URI               string    `json:"uri"`
	Prefix            string    `json:"prefix"`
	LocalName         string    `json:"local_name"`
	Datatype          string    `json:"datatype,omitempty"`
	Position          int       `json:"position"`
	ClassCode         string    `json:"class_code,omitempty"`
	InstanceID        string    `json:"instance_id,omitempty"`
	PathNode          string    `json:"path_node,omitempty"`
	PathNodeID        string    `json:"path_node_id,omitempty"`
	OntologyVersionID string    `json:"ontology_version_id,omitempty"`
	AdditionalTypes   []TypeRef `json:"additional_types,omitempty"`
	SubPropertyOf     string    `json:"sub_property_of,omitempty"`
}

// TypeRef identifies an ontology class used as an additional path-element type.
type TypeRef struct {
	URI       string `json:"uri"`
	Prefix    string `json:"prefix"`
	LocalName string `json:"local_name"`
	ClassCode string `json:"class_code,omitempty"`
}

// PrefixedName returns the namespaced form, e.g. "crm:E21_Person".
func (pe PathElement) PrefixedName() string {
	primary := pe.LocalName
	if pe.Prefix != "" {
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

// PrefixedName returns the namespaced form for a TypeRef.
func (t TypeRef) PrefixedName() string {
	if t.Prefix == "" {
		return t.LocalName
	}
	return t.Prefix + ":" + t.LocalName
}

// AllTypes returns the primary and additional class references for this element.
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
