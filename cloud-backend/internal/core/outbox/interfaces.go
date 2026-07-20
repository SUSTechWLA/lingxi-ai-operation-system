package outbox

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
)

// EventSaver defines the interface for saving events to the outbox.
type EventSaver interface {
	SaveEvent(ctx context.Context, aggregateType, aggregateID, eventType string, event eventbus.Event) error
}

// AtomicNodeReadySaver makes an executable node READY and records its outbox
// event in the same database transaction. The bool reports whether this caller
// won the conditional CREATED -> READY transition.
type AtomicNodeReadySaver interface {
	SaveNodeReadyEvent(ctx context.Context, nodeID, idempotencyKey string, event eventbus.Event) (bool, error)
}

// EventPublisher defines the interface for publishing events to the message broker.
// eventbus.Producer satisfies this via its Publish method.
type EventPublisher interface {
	Publish(topic, key string, event eventbus.Event) error
}

// OutboxStore defines the DB operations Relay needs. Extracted as an interface
// so Relay can be tested with in-memory mocks instead of a real PostgreSQL pool.
type OutboxStore interface {
	// FetchPending returns up to limit unprocessed events ordered by id,
	// locking the rows to prevent concurrent relay instances from competing.
	FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error)

	// Delete removes a successfully relayed event from the outbox.
	Delete(ctx context.Context, id int64) error

	// IncrementRetry bumps retry_count and records the error message.
	IncrementRetry(ctx context.Context, id int64, errMsg string) error

	// MoveToDLQ moves an event that exceeded max retries to the dead-letter table.
	MoveToDLQ(ctx context.Context, entry OutboxEntry, errMsg string) error
}

// OutboxSaver wraps the SaveEvent function to implement EventSaver interface.
type OutboxSaver struct {
	pool *pgxpool.Pool
}

// NewOutboxSaver creates an OutboxSaver with a database pool.
func NewOutboxSaver(pool *pgxpool.Pool) *OutboxSaver {
	return &OutboxSaver{pool: pool}
}

func (s *OutboxSaver) SaveEvent(ctx context.Context, aggregateType, aggregateID, eventType string, event eventbus.Event) error {
	return SaveEvent(ctx, s.pool, aggregateType, aggregateID, eventType, event)
}

func (s *OutboxSaver) SaveNodeReadyEvent(ctx context.Context, nodeID, idempotencyKey string, event eventbus.Event) (bool, error) {
	return SaveNodeReadyEvent(ctx, s.pool, nodeID, idempotencyKey, event)
}
