package domain

import (
	"context"
	"testing"
)

func TestSimpleEventBus_DispatchesToSubscribers(t *testing.T) {
	bus := NewSimpleEventBus()
	var gotA, gotB int
	bus.Subscribe("X", func(_ context.Context, _ Event) { gotA++ })
	bus.Subscribe("X", func(_ context.Context, _ Event) { gotB++ })
	bus.Subscribe("Y", func(_ context.Context, _ Event) { t.Error("Y handler must not fire on X") })

	bus.Publish(context.Background(), Event{Type: "X"})

	if gotA != 1 || gotB != 1 {
		t.Fatalf("want both handlers fired once, got a=%d b=%d", gotA, gotB)
	}
}

func TestSimpleEventBus_HandlerMaySubscribeDuringPublish(t *testing.T) {
	bus := NewSimpleEventBus()
	bus.Subscribe("X", func(ctx context.Context, _ Event) {
		bus.Subscribe("X", func(context.Context, Event) {}) // must not deadlock
	})
	done := make(chan struct{})
	go func() { bus.Publish(context.Background(), Event{Type: "X"}); close(done) }()
	select {
	case <-done:
	case <-context.Background().Done():
	}
	// If Publish held the write-lock during handler calls this would deadlock; the
	// test simply completing proves the copy-under-lock pattern.
}
