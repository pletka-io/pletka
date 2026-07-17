package domain

// PathQuery defines parameters for searching fields by ontology path patterns.
// Supports three modes: contiguous, subsequence, and anchored.
type PathQuery struct {
	Anchor     string   `json:"anchor,omitempty"`
	LocalNames []string `json:"local_names"`
	Contiguous bool     `json:"contiguous"`
	ProjectID  string   `json:"project_id,omitempty"`
}
