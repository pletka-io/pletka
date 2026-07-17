package domain

import "context"

// Ontology-related event type constants.
const (
	// EventOntologyVersionImported is published after a successful version import.
	// EntityID carries the imported version's ID; use it to invalidate cache entries
	// for that specific version.
	EventOntologyVersionImported = "OntologyVersionImported"

	// EventProjectOntologyVersionsChanged is published when a project's set of linked
	// ontology versions changes (Create, Update, Delete). ProjectID carries the project;
	// use it to drop all cached live indexes for that project.
	EventProjectOntologyVersionsChanged = "ProjectOntologyVersionsChanged"
)

// EventBus provides fire-and-forget cross-service communication.
type EventBus interface {
	Publish(ctx context.Context, event Event)
	Subscribe(eventType string, handler EventHandler)
}

// EventHandler is a function that processes a domain event.
type EventHandler func(ctx context.Context, event Event)

// Event represents a domain event published through the EventBus.
type Event struct {
	Type      string
	ProjectID string
	EntityID  string
	Payload   map[string]any
}

// NoOpEventBus is a placeholder that does nothing.
// Use it during development or when event handling is not yet needed.
type NoOpEventBus struct{}

// Compile-time check that NoOpEventBus implements EventBus.
var _ EventBus = NoOpEventBus{}

func (NoOpEventBus) Publish(_ context.Context, _ Event)   {}
func (NoOpEventBus) Subscribe(_ string, _ EventHandler)    {}
