package outbox

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/tangying-ai-operation-system/internal/eventbus"
)

// EventSaver defines the interface for saving events to the outbox.
type EventSaver interface {
	SaveEvent(ctx context.Context, aggregateType, aggregateID, eventType string, event eventbus.Event) error
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
