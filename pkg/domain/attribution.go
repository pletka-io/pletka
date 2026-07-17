package domain

import "time"

// Attribution is a project-credits row recording that an actor played
// a role (author / funder / adopter) in a project. Distinct from
// weave_memberships (authorisation) and weave_project_actors
// (institutional ownership).
//
// The Store interface lives in the attribution slice
// (pkg/weave/attribution). This package only carries the value type
// because cross-slice consumers (e.g. projectpage) need to read it.
type Attribution struct {
	ProjectID string    `json:"project_id"`
	ActorID   string    `json:"actor_id"`
	Kind      string    `json:"kind"` // "author" | "funder" | "adopter"
	Position  int       `json:"position"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	// Joined display columns.
	ActorName string `json:"actor_name,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
}
