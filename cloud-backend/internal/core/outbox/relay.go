package outbox

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
)

// DefaultRelayConfig returns sane defaults for the outbox relay.
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		PollInterval:     100 * time.Millisecond,
		MaxRetries:       10,
		BackoffMax:       30 * time.Second,
		BackoffMultiplier: 2.0,
		BatchSize:        100,
	}
}

// RelayConfig tunes the relay behaviour.
type RelayConfig struct {
	PollInterval     time.Duration // base interval between polls
	MaxRetries       int           // moves to DLQ after this many failed attempts
	BackoffMax       time.Duration // cap for exponential backoff
	BackoffMultiplier float64       // multiplier when Kafka is down
	BatchSize        int           // max rows to fetch per poll
}

type OutboxEntry struct {
	ID            int64       `json:"id"`
	AggregateType string      `json:"aggregateType"`
	AggregateID   string      `json:"aggregateId"`
	EventType     string      `json:"eventType"`
	Payload       interface{} `json:"payload"`
	RetryCount    int         `json:"retryCount"`
	LastError     string      `json:"lastError,omitempty"`
	CreatedAt     time.Time   `json:"createdAt"`
}

// Relay reads events from the outbox table and publishes them to Kafka.
// It supports retry with exponential backoff, a dead-letter queue, and
// concurrent-safe row locking (FOR UPDATE SKIP LOCKED).
type Relay struct {
	store    OutboxStore
	pub      EventPublisher
	cfg      RelayConfig
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewRelay creates a Relay backed by a real PostgreSQL pool.
func NewRelay(pool *pgxpool.Pool, producer *eventbus.Producer, cfg RelayConfig) *Relay {
	return &Relay{
		store: &pgStore{pool: pool},
		pub:   producer,
		cfg:   cfg,
	}
}

// NewRelayWithStore creates a Relay with custom store and publisher (useful for testing).
func NewRelayWithStore(store OutboxStore, pub EventPublisher, cfg RelayConfig) *Relay {
	return &Relay{store: store, pub: pub, cfg: cfg}
}

// Start begins the relay loop in a background goroutine.
func (r *Relay) Start(ctx context.Context) {
	relayCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.wg.Add(1)
	go r.run(relayCtx)
	zap.L().Info("Outbox relay started",
		zap.Duration("pollInterval", r.cfg.PollInterval),
		zap.Int("maxRetries", r.cfg.MaxRetries),
		zap.Duration("backoffMax", r.cfg.BackoffMax),
	)
}

// Stop signals the relay to shut down and waits for in-flight work to finish.
func (r *Relay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	zap.L().Info("Outbox relay stopped")
}

func (r *Relay) run(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	currentInterval := r.cfg.PollInterval

	for {
		select {
		case <-ctx.Done():
			// Drain remaining events before exit
			r.processPending(ctx)
			return
		case <-ticker.C:
			hadFailure := r.processPending(ctx)
			if hadFailure {
				// Exponential backoff: double the interval, cap at max
				currentInterval = time.Duration(float64(currentInterval) * r.cfg.BackoffMultiplier)
				if currentInterval > r.cfg.BackoffMax {
					currentInterval = r.cfg.BackoffMax
				}
				ticker.Reset(currentInterval)
			} else {
				// Reset to base poll interval on success
				currentInterval = r.cfg.PollInterval
				ticker.Reset(currentInterval)
			}
		}
	}
}

// processPending fetches and relays a batch. Returns true if any publish
// failed (so the caller can apply backoff).
func (r *Relay) processPending(ctx context.Context) bool {
	entries, err := r.store.FetchPending(ctx, r.cfg.BatchSize)
	if err != nil {
		zap.L().Error("Outbox: failed to fetch pending events", zap.Error(err))
		return true
	}

	if len(entries) == 0 {
		return false // nothing to do → no failure
	}

	var relayed, failed, dead int
	hadFailure := false

	for _, entry := range entries {
		event, rawPayload, err := r.unmarshalEvent(entry)
		if err != nil {
			// Malformed JSON — move directly to DLQ
			zap.L().Error("Outbox: unmarshal failed, moving to DLQ",
				zap.Int64("id", entry.ID),
				zap.Error(err),
			)
			if moveErr := r.store.MoveToDLQ(ctx, entry, err.Error()); moveErr != nil {
				zap.L().Error("Outbox: failed to move event to DLQ", zap.Int64("id", entry.ID), zap.Error(moveErr))
			}
			dead++
			continue
		}

		key := entry.AggregateID
		if key == "" {
			key = event.TaskID
		}

		if err := r.pub.Publish(entry.EventType, key, event); err != nil {
			zap.L().Error("Outbox: publish failed",
				zap.Int64("id", entry.ID),
				zap.String("eventType", entry.EventType),
				zap.Int("retryCount", entry.RetryCount),
				zap.Error(err),
			)
			hadFailure = true

			// Oversized messages cannot be retried — move to DLQ
			if isOversized(err) {
				zap.L().Warn("Outbox: oversized event → DLQ",
					zap.Int64("id", entry.ID),
					zap.String("eventType", entry.EventType),
					zap.Int("payloadSize", len(rawPayload)),
				)
				if moveErr := r.store.MoveToDLQ(ctx, entry, err.Error()); moveErr != nil {
					zap.L().Error("Outbox: failed to move oversized event to DLQ", zap.Int64("id", entry.ID), zap.Error(moveErr))
				}
				dead++
				continue
			}

			// Check retry limit
			if entry.RetryCount >= r.cfg.MaxRetries {
				zap.L().Warn("Outbox: max retries exceeded → DLQ",
					zap.Int64("id", entry.ID),
					zap.String("eventType", entry.EventType),
					zap.Int("retryCount", entry.RetryCount),
				)
				if moveErr := r.store.MoveToDLQ(ctx, entry, err.Error()); moveErr != nil {
					zap.L().Error("Outbox: failed to move event to DLQ", zap.Int64("id", entry.ID), zap.Error(moveErr))
				}
				dead++
				continue
			}

			// Increment retry count, keep in outbox for next attempt
			if incrErr := r.store.IncrementRetry(ctx, entry.ID, err.Error()); incrErr != nil {
				zap.L().Error("Outbox: failed to increment retry count", zap.Int64("id", entry.ID), zap.Error(incrErr))
			}
			failed++
			continue
		}

		// Success — remove from outbox
		if delErr := r.store.Delete(ctx, entry.ID); delErr != nil {
			zap.L().Error("Outbox: failed to delete relayed event", zap.Int64("id", entry.ID), zap.Error(delErr))
			// Event was published but we couldn't delete — duplicate possible on next poll.
			// This is acceptable (at-least-once semantics).
		}
		relayed++
	}

	if relayed+failed+dead > 0 {
		zap.L().Info("Outbox: batch complete",
			zap.Int("relayed", relayed),
			zap.Int("failed_retry", failed),
			zap.Int("dead", dead),
		)
	}

	return hadFailure
}

func (r *Relay) unmarshalEvent(entry OutboxEntry) (eventbus.Event, []byte, error) {
	switch v := entry.Payload.(type) {
	case []byte:
		var event eventbus.Event
		if err := json.Unmarshal(v, &event); err != nil {
			return eventbus.Event{}, nil, err
		}
		return event, v, nil
	default:
		// Re-marshal to get the raw bytes (for size checks)
		raw, err := json.Marshal(entry.Payload)
		if err != nil {
			return eventbus.Event{}, nil, err
		}
		var event eventbus.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return eventbus.Event{}, nil, err
		}
		return event, raw, nil
	}
}

func isOversized(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Message larger than configured") ||
		strings.Contains(msg, "MaxMessageBytes") ||
		strings.Contains(msg, "message too large")
}

// ── PostgreSQL-backed OutboxStore ────────────────────────────────────────

type pgStore struct {
	pool *pgxpool.Pool
}

func (s *pgStore) FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, aggregate_type, aggregate_id, event_type, payload, retry_count, COALESCE(last_error,''), created_at
		 FROM outbox ORDER BY id ASC LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []OutboxEntry
	for rows.Next() {
		var e OutboxEntry
		var payload []byte
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType, &payload, &e.RetryCount, &e.LastError, &e.CreatedAt); err != nil {
			zap.L().Error("Outbox: scan error", zap.Error(err))
			continue
		}
		e.Payload = payload
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *pgStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM outbox WHERE id=$1`, id)
	return err
}

func (s *pgStore) IncrementRetry(ctx context.Context, id int64, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE outbox SET retry_count = retry_count + 1, last_error = $2 WHERE id = $1`,
		id, errMsg,
	)
	return err
}

func (s *pgStore) MoveToDLQ(ctx context.Context, entry OutboxEntry, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO outbox_dlq (aggregate_type, aggregate_id, event_type, payload, retry_count, last_error, original_id, created_at, dead_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
		entry.AggregateType, entry.AggregateID, entry.EventType, entry.Payload,
		entry.RetryCount, errMsg, entry.ID,
	)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM outbox WHERE id=$1`, entry.ID)
	return err
}

// Compile-time check
var _ OutboxStore = (*pgStore)(nil)

// SaveEvent writes an event to the outbox table (to be relayed to Kafka).
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
