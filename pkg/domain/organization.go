package domain

import "time"

// Organization is the canonical org-like actor surfaced through /orgs.
// It is stored in weave_actors with type="organization".
type Organization struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DisplayName string    `json:"display_name"`
	SystemName  *string   `json:"system_name,omitempty"`
	Acronym     *string   `json:"acronym,omitempty"`
	Country     *string   `json:"country,omitempty"`
	Website     *string   `json:"website,omitempty"`
	Slug        string    `json:"slug"`
	Visibility  string    `json:"visibility"`
	CreatedByID *string   `json:"created_by_id,omitempty"`
}
