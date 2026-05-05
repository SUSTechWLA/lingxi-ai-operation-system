package eventbus

// EventPublisher defines the interface for publishing events.
// Used by services that need to publish events via the outbox pattern.
type EventPublisher interface {
	Publish(topic, key string, event Event) error
}

// Compile-time check that Producer satisfies EventPublisher.
var _ EventPublisher = (*Producer)(nil)
