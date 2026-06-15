package outbox

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
)

type OutboxEntry struct {
	ID            int64       `json:"id"`
	AggregateType string      `json:"aggregateType"`
	AggregateID   string      `json:"aggregateId"`
	EventType     string      `json:"eventType"`
	Payload       interface{} `json:"payload"`
	CreatedAt     time.Time   `json:"createdAt"`
}

type Relay struct {
	pool     *pgxpool.Pool
	producer *eventbus.Producer
	cancel   context.CancelFunc
}

func NewRelay(pool *pgxpool.Pool, producer *eventbus.Producer) *Relay {
	return &Relay{pool: pool, producer: producer}
}

func (r *Relay) Start(ctx context.Context) {
	relayCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	go r.run(relayCtx)
	zap.L().Info("Outbox relay started")
}

func (r *Relay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Relay) run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processPending(ctx)
		}
	}
}

func (r *Relay) processPending(ctx context.Context) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at
		 FROM outbox ORDER BY id ASC LIMIT 100`,
	)
	if err != nil {
		zap.L().Error("Outbox: failed to query pending events", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var entry OutboxEntry
		var payload []byte

		if err := rows.Scan(&entry.ID, &entry.AggregateType, &entry.AggregateID, &entry.EventType, &payload, &entry.CreatedAt); err != nil {
			zap.L().Error("Outbox: failed to scan entry", zap.Error(err))
			continue
		}

		var event eventbus.Event
		if err := json.Unmarshal(payload, &event); err != nil {
			zap.L().Error("Outbox: failed to unmarshal event", zap.Error(err))
			// Delete malformed entries
			_, _ = r.pool.Exec(ctx, `DELETE FROM outbox WHERE id=$1`, entry.ID)
			continue
		}

		key := entry.AggregateID
		if key == "" {
			key = event.TaskID
		}

		if err := r.producer.Publish(entry.EventType, key, event); err != nil {
			zap.L().Error("Outbox: failed to publish event", zap.Error(err))
			// If message is too large for Kafka, delete it to unblock the queue
			if strings.Contains(err.Error(), "Message larger than configured") ||
				strings.Contains(err.Error(), "MaxMessageBytes") {
				zap.L().Warn("Outbox: deleting oversized event",
					zap.Int64("id", entry.ID),
					zap.String("eventType", entry.EventType),
					zap.Int("payloadSize", len(payload)))
				_, _ = r.pool.Exec(ctx, `DELETE FROM outbox WHERE id=$1`, entry.ID)
				continue
			}
			return // Stop processing, will retry next tick
		}

		_, _ = r.pool.Exec(ctx, `DELETE FROM outbox WHERE id=$1`, entry.ID)
		zap.L().Debug("Outbox: relayed event", zap.String("eventType", entry.EventType), zap.Int64("id", entry.ID))
	}
}

// SaveEvent writes an event to the outbox table (to be relayed to Kafka)
func SaveEvent(ctx context.Context, pool *pgxpool.Pool, aggregateType, aggregateID, eventType string, event eventbus.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload)
		 VALUES ($1, $2, $3, $4)`,
		aggregateType, aggregateID, eventType, payload,
	)
	return err
}
