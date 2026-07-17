package domain

import (
	"context"
	"sync"
)

// SimpleEventBus is a synchronous in-process EventBus. Handlers run
// inline on Publish; a slow handler is the handler's problem (it should
// spawn its own goroutine). Cache invalidation handlers complete in
// microseconds, so synchronous dispatch is correct here.
type SimpleEventBus struct {
	mu       sync.RWMutex
	handlers map[string][]EventHandler
}

// NewSimpleEventBus returns an empty bus ready for Subscribe/Publish.
func NewSimpleEventBus() *SimpleEventBus {
	return &SimpleEventBus{handlers: map[string][]EventHandler{}}
}

// Subscribe registers handler for the given event type.
func (b *SimpleEventBus) Subscribe(eventType string, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish dispatches event to every handler for its type, synchronously.
// Handlers are copied under the read-lock so a handler may Subscribe
// without deadlocking.
func (b *SimpleEventBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	hs := append([]EventHandler(nil), b.handlers[event.Type]...)
	b.mu.RUnlock()
	for _, h := range hs {
		h(ctx, event)
	}
}

var _ EventBus = (*SimpleEventBus)(nil)
