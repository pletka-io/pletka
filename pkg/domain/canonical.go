package domain

// AuditableEntity is implemented by domain entities that participate in
// change tracking and git-backed storage.
type AuditableEntity interface {
	EntityType() string
	EntityID() string
	FilePath() string
	CanonicalContent() ([]byte, error)
}
