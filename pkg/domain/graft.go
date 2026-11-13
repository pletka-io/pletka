package domain

// GraftOverride changes one grafted copy of a field inside one model,
// addressed by the chain of field semantic ids from the model down through
// the stubs ("LAF.177/LAF.100/LAF.12"). It never touches the collection the
// copy comes from.
type GraftOverride struct {
	Address   string `json:"address"`
	IsHidden  bool   `json:"is_hidden,omitempty"`
	MaxOccurs *int   `json:"max_occurs,omitempty"`
	// MaxOccursSet is true when the override sets max_occurs; with MaxOccurs
	// nil it means unbounded.
	MaxOccursSet bool         `json:"max_occurs_set,omitempty"`
	DisplayName  Translations `json:"display_name,omitempty"`
}
