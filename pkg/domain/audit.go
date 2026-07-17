package domain

// ChangeLogEntry represents a single entity mutation within a ChangeSet.
type ChangeLogEntry struct {
	EntityType      string
	EntityID        string
	Operation       string // "create", "update", "delete"
	ProjectID       string
	FilePath        string
	Payload         []byte // nil for delete
	PreviousPayload []byte // nil for create
}
