package release

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("release: not found")

type Release struct {
	ProjectID       string     `json:"project_id"`
	Version         string     `json:"version"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	CreatedAt       time.Time  `json:"created_at"`
	CreatedByID     string     `json:"created_by_id"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	ArchivedMessage string     `json:"archived_message,omitempty"`
}
