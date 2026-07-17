package domain

// Category is a display grouping for fields within a project.
// Categories carry no ontological meaning — they organise fields for human consumption
// (tabs in a form, sections in documentation).
type Category struct {
	Entity
	Origin         Origin `json:"origin,omitempty"`
	CanonicalOrder int `json:"canonical_order"`
}
