package eventbus

import "context"

// EventPublisher defines the interface for publishing events.
// Used by services that need to publish events via the outbox pattern.
type EventPublisher interface {
	Publish(topic, key string, event Event) error
}

type ContextEventPublisher interface {
	PublishContext(context.Context, string, string, Event) error
}

// PublishWithContext uses the context-aware publisher when available and
// safely adapts legacy publishers by enriching a per-call event copy.
func PublishWithContext(ctx context.Context, publisher EventPublisher, topic, key string, event Event) error {
	if contextual, ok := publisher.(ContextEventPublisher); ok {
		return contextual.PublishContext(ctx, topic, key, event)
	}
	return publisher.Publish(topic, key, eventWithCorrelation(ctx, event))
}

// Compile-time check that Producer satisfies EventPublisher.
var _ EventPublisher = (*Producer)(nil)
var _ ContextEventPublisher = (*Producer)(nil)
