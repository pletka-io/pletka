package auth

// Principal is the request-scoped authenticated user identity used by the
// weave auth path. It is intentionally small and decoupled from persistence
// models.
type Principal struct {
	ActorID     string
	Slug        string
	Email       string
	DisplayName string
	Role        string
	IsActive    bool
}
