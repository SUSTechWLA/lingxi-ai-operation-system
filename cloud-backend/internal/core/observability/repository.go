package observability

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPullLimit = 100
	maxPullLimit     = 500
	maxAckEventIDs   = 500
	maxCursorBytes   = 2048
	relayLifetime    = 72 * time.Hour
	maxRelayIDBytes  = 128
)

var (
	ErrEventConflict   = errors.New("observability event identity conflict")
	ErrInvalidCursor   = errors.New("invalid observability cursor")
	ErrTooManyEventIDs = errors.New("too many event IDs")
	ErrSummaryNotFound = errors.New("observability run summary not found")
	trustedUserPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

type EventPage struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"nextCursor,omitempty"`
}

type RunVersions struct {
	AppVersion             string `json:"appVersion,omitempty"`
	GitCommit              string `json:"gitCommit,omitempty"`
	WorkflowVersion        string `json:"workflowVersion,omitempty"`
	ToolRegistrySnapshotID string `json:"toolRegistrySnapshotId,omitempty"`
	PromptTemplateVersion  string `json:"promptTemplateVersion,omitempty"`
}

type RunSummary struct {
	RunID             string          `json:"runId"`
	Status            ExecutionStatus `json:"status"`
	DurationMs        *int64          `json:"durationMs,omitempty"`
	ErrorFingerprints []string        `json:"errorFingerprints"`
	Correlation       Correlation     `json:"correlation"`
	Versions          RunVersions     `json:"versions"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

type eventRows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close()
	Err() error
}

type relayDB interface {
	QueryRow(context.Context, string, ...interface{}) rowScanner
	Query(context.Context, string, ...interface{}) (eventRows, error)
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
}

type pgxRelayDB struct{ pool *pgxpool.Pool }

func (db pgxRelayDB) QueryRow(ctx context.Context, query string, args ...interface{}) rowScanner {
	return db.pool.QueryRow(ctx, query, args...)
}

func (db pgxRelayDB) Query(ctx context.Context, query string, args ...interface{}) (eventRows, error) {
	return db.pool.Query(ctx, query, args...)
}

func (db pgxRelayDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, query, args...)
}

type Repository struct{ db relayDB }

func NewRepository(pool *pgxpool.Pool) *Repository {
	return newRepositoryWithRelayDB(pgxRelayDB{pool: pool})
}

func newRepositoryWithRelayDB(db relayDB) *Repository {
	return &Repository{db: db}
}

const saveSummarySQL = `
WITH accepted AS (
	INSERT INTO observability_event_outbox (
		event_id, user_id, run_id, trace_id, occurred_at, redacted_payload, ingested_at, expires_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	ON CONFLICT (event_id) DO UPDATE SET event_id=observability_event_outbox.event_id
	WHERE observability_event_outbox.user_id=EXCLUDED.user_id
	  AND observability_event_outbox.redacted_payload=EXCLUDED.redacted_payload
	RETURNING event_id
), summary AS (
	INSERT INTO observability_run_summaries (
		user_id, run_id, status, duration_ms, error_fingerprints, correlation, versions, updated_at
	)
	SELECT $2,$3,$9,$10,$11,$12,$13,$5 WHERE EXISTS (SELECT 1 FROM accepted)
	ON CONFLICT (user_id, run_id) DO UPDATE SET
		status=CASE WHEN EXCLUDED.updated_at >= observability_run_summaries.updated_at
			THEN EXCLUDED.status ELSE observability_run_summaries.status END,
		duration_ms=CASE WHEN EXCLUDED.updated_at >= observability_run_summaries.updated_at
			THEN EXCLUDED.duration_ms ELSE observability_run_summaries.duration_ms END,
		error_fingerprints=(
			SELECT COALESCE(array_agg(DISTINCT fingerprint), ARRAY[]::TEXT[])
			FROM unnest(observability_run_summaries.error_fingerprints || EXCLUDED.error_fingerprints) AS fingerprint
		),
		correlation=CASE WHEN EXCLUDED.updated_at >= observability_run_summaries.updated_at
			THEN EXCLUDED.correlation ELSE observability_run_summaries.correlation END,
		versions=CASE WHEN EXCLUDED.updated_at >= observability_run_summaries.updated_at
			THEN EXCLUDED.versions ELSE observability_run_summaries.versions END,
		updated_at=GREATEST(observability_run_summaries.updated_at, EXCLUDED.updated_at)
)
SELECT EXISTS (SELECT 1 FROM accepted)`

func (repository *Repository) SaveSummary(ctx context.Context, userID string, event Event) error {
	if !trustedUserPattern.MatchString(userID) {
		return errors.New("trusted user ID is required")
	}
	redacted := Redact(event)
	if ContainsSecret(redacted) {
		return errors.New("observability event contains secret material")
	}
	if err := redacted.Validate(); err != nil {
		return fmt.Errorf("validate redacted observability event: %w", err)
	}
	if redacted.OccurredAt.IsZero() || redacted.IngestedAt.IsZero() {
		return errors.New("observability event timestamps are required")
	}
	if len(redacted.EventID) > maxRelayIDBytes || len(redacted.Correlation.TraceID) > maxRelayIDBytes {
		return errors.New("observability event identity exceeds storage bounds")
	}
	runID, err := eventRunID(redacted)
	if err != nil {
		return err
	}
	if len(runID) > maxRelayIDBytes {
		return errors.New("observability run identity exceeds storage bounds")
	}
	payload, err := json.Marshal(redacted)
	if err != nil {
		return fmt.Errorf("marshal redacted observability event: %w", err)
	}
	summary := summaryFromEvent(runID, redacted)
	correlationJSON, err := json.Marshal(summary.Correlation)
	if err != nil {
		return fmt.Errorf("marshal observability correlation: %w", err)
	}
	versionsJSON, err := json.Marshal(summary.Versions)
	if err != nil {
		return fmt.Errorf("marshal observability versions: %w", err)
	}

	var accepted bool
	err = repository.db.QueryRow(
		ctx,
		saveSummarySQL,
		redacted.EventID,
		userID,
		runID,
		redacted.Correlation.TraceID,
		redacted.OccurredAt,
		payload,
		redacted.IngestedAt,
		redacted.IngestedAt.Add(relayLifetime),
		summary.Status,
		summary.DurationMs,
		summary.ErrorFingerprints,
		correlationJSON,
		versionsJSON,
	).Scan(&accepted)
	if err != nil {
		return fmt.Errorf("save observability summary: %w", err)
	}
	if !accepted {
		return ErrEventConflict
	}
	return nil
}

func eventRunID(event Event) (string, error) {
	workflowRunID := event.Correlation.WorkflowRunID
	agentRunID := event.Correlation.AgentRunID
	if workflowRunID != "" && agentRunID != "" {
		return "", errors.New("observability event has ambiguous run identity")
	}
	switch {
	case strings.HasPrefix(string(event.EventType), "workflow."):
		if workflowRunID == "" {
			return "", errors.New("workflow event requires workflow run identity")
		}
		return workflowRunID, nil
	case strings.HasPrefix(string(event.EventType), "agent."):
		if agentRunID == "" {
			return "", errors.New("agent event requires agent run identity")
		}
		return agentRunID, nil
	case workflowRunID != "":
		return workflowRunID, nil
	case agentRunID != "":
		return agentRunID, nil
	default:
		return "", errors.New("observability event requires run identity")
	}
}

func summaryFromEvent(runID string, event Event) RunSummary {
	fingerprints := []string{}
	if event.Error != nil && event.Error.Fingerprint != "" {
		fingerprints = append(fingerprints, event.Error.Fingerprint)
	}
	return RunSummary{
		RunID:             runID,
		Status:            event.Execution.Status,
		DurationMs:        cloneInt64(event.Execution.DurationMs),
		ErrorFingerprints: fingerprints,
		Correlation:       event.Correlation,
		Versions: RunVersions{
			AppVersion:             event.Runtime.AppVersion,
			GitCommit:              event.Runtime.GitCommit,
			WorkflowVersion:        event.Runtime.WorkflowVersion,
			ToolRegistrySnapshotID: event.Runtime.ToolRegistrySnapshotID,
			PromptTemplateVersion:  event.Runtime.PromptTemplateVersion,
		},
		UpdatedAt: event.OccurredAt,
	}
}

const pullEventsSQL = `
SELECT event_id, occurred_at, redacted_payload
FROM observability_event_outbox
WHERE user_id=$1
  AND (occurred_at, event_id) > ($2,$3)
  AND delivered_at IS NULL
  AND expires_at > NOW()
ORDER BY occurred_at ASC, event_id ASC
LIMIT $4`

func (repository *Repository) Pull(ctx context.Context, userID, cursor string, limit int) (EventPage, error) {
	if !trustedUserPattern.MatchString(userID) {
		return EventPage{}, errors.New("trusted user ID is required")
	}
	limit = normalizedPullLimit(limit)
	cursorTime := time.Time{}
	cursorEventID := ""
	if cursor != "" {
		decoded, err := decodeCursor(cursor)
		if err != nil {
			return EventPage{}, err
		}
		cursorTime, cursorEventID = decoded.OccurredAt, decoded.EventID
	}
	rows, err := repository.db.Query(ctx, pullEventsSQL, userID, cursorTime, cursorEventID, limit+1)
	if err != nil {
		return EventPage{}, fmt.Errorf("pull observability events: %w", err)
	}
	defer rows.Close()

	page := EventPage{Events: []Event{}}
	for rows.Next() {
		var eventID string
		var occurredAt time.Time
		var payload []byte
		if err := rows.Scan(&eventID, &occurredAt, &payload); err != nil {
			return EventPage{}, fmt.Errorf("scan observability event: %w", err)
		}
		if len(page.Events) == limit {
			continue
		}
		event, err := decodePersistedEvent(payload)
		if err != nil {
			return EventPage{}, err
		}
		if event.EventID != eventID || !event.OccurredAt.Equal(occurredAt) {
			return EventPage{}, errors.New("observability event row identity mismatch")
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, fmt.Errorf("iterate observability events: %w", err)
	}
	if len(page.Events) == limit {
		last := page.Events[len(page.Events)-1]
		page.NextCursor, err = encodeCursor(last.OccurredAt, last.EventID)
		if err != nil {
			return EventPage{}, err
		}
	}
	return page, nil
}

func normalizedPullLimit(limit int) int {
	if limit <= 0 {
		return defaultPullLimit
	}
	if limit > maxPullLimit {
		return maxPullLimit
	}
	return limit
}

type relayCursor struct {
	Version    int    `json:"v"`
	OccurredAt string `json:"t"`
	EventID    string `json:"e"`
}

type decodedRelayCursor struct {
	OccurredAt time.Time
	EventID    string
}

func encodeCursor(occurredAt time.Time, eventID string) (string, error) {
	payload, err := json.Marshal(relayCursor{
		Version:    1,
		OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano),
		EventID:    eventID,
	})
	if err != nil {
		return "", fmt.Errorf("encode observability cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(value string) (decodedRelayCursor, error) {
	if value == "" || len(value) > maxCursorBytes {
		return decodedRelayCursor{}, ErrInvalidCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(payload) > maxCursorBytes {
		return decodedRelayCursor{}, ErrInvalidCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var cursor relayCursor
	if err := decoder.Decode(&cursor); err != nil {
		return decodedRelayCursor{}, ErrInvalidCursor
	}
	if err := requireJSONEOF(decoder); err != nil {
		return decodedRelayCursor{}, ErrInvalidCursor
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, cursor.OccurredAt)
	if err != nil || cursor.Version != 1 || !validRelayEventID(cursor.EventID) {
		return decodedRelayCursor{}, ErrInvalidCursor
	}
	return decodedRelayCursor{OccurredAt: occurredAt, EventID: cursor.EventID}, nil
}

func decodePersistedEvent(payload []byte) (Event, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("decode redacted observability event: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Event{}, fmt.Errorf("decode redacted observability event: %w", err)
	}
	if ContainsSecret(event) {
		return Event{}, errors.New("persisted observability event contains secret material")
	}
	if err := event.Validate(); err != nil {
		return Event{}, fmt.Errorf("validate persisted observability event: %w", err)
	}
	return event, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

const acknowledgeEventsSQL = `
UPDATE observability_event_outbox
SET delivered_at=COALESCE(delivered_at, NOW())
WHERE user_id=$1
  AND event_id = ANY($2)
  AND delivered_at IS NULL
  AND expires_at > NOW()`

func (repository *Repository) Acknowledge(ctx context.Context, userID string, eventIDs []string) (int64, error) {
	if !trustedUserPattern.MatchString(userID) {
		return 0, errors.New("trusted user ID is required")
	}
	if len(eventIDs) == 0 {
		return 0, errors.New("event IDs are required")
	}
	if len(eventIDs) > maxAckEventIDs {
		return 0, ErrTooManyEventIDs
	}
	unique := make([]string, 0, len(eventIDs))
	seen := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if !validRelayEventID(eventID) {
			return 0, errors.New("invalid event ID")
		}
		if _, exists := seen[eventID]; exists {
			continue
		}
		seen[eventID] = struct{}{}
		unique = append(unique, eventID)
	}
	tag, err := repository.db.Exec(ctx, acknowledgeEventsSQL, userID, unique)
	if err != nil {
		return 0, fmt.Errorf("acknowledge observability events: %w", err)
	}
	return tag.RowsAffected(), nil
}

const runSummarySQL = `
SELECT run_id, status, duration_ms, error_fingerprints, correlation, versions, updated_at
FROM observability_run_summaries
WHERE user_id=$1 AND run_id=$2`

func (repository *Repository) RunSummary(ctx context.Context, userID, runID string) (RunSummary, error) {
	if !trustedUserPattern.MatchString(userID) {
		return RunSummary{}, errors.New("trusted user ID is required")
	}
	if !validRunID(runID) {
		return RunSummary{}, errors.New("invalid run ID")
	}
	var summary RunSummary
	var correlationJSON []byte
	var versionsJSON []byte
	err := repository.db.QueryRow(ctx, runSummarySQL, userID, runID).Scan(
		&summary.RunID,
		&summary.Status,
		&summary.DurationMs,
		&summary.ErrorFingerprints,
		&correlationJSON,
		&versionsJSON,
		&summary.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunSummary{}, ErrSummaryNotFound
	}
	if err != nil {
		return RunSummary{}, fmt.Errorf("read observability run summary: %w", err)
	}
	if summary.RunID != runID {
		return RunSummary{}, errors.New("observability summary row identity mismatch")
	}
	if err := strictJSON(correlationJSON, &summary.Correlation); err != nil {
		return RunSummary{}, fmt.Errorf("decode observability summary correlation: %w", err)
	}
	if err := strictJSON(versionsJSON, &summary.Versions); err != nil {
		return RunSummary{}, fmt.Errorf("decode observability summary versions: %w", err)
	}
	if ContainsSecret(summary) {
		return RunSummary{}, errors.New("observability summary contains secret material")
	}
	if err := validateRunSummary(summary); err != nil {
		return RunSummary{}, err
	}
	return summary, nil
}

func strictJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func validRunID(runID string) bool {
	return len(runID) <= maxRelayIDBytes &&
		(workflowRunIDPattern.MatchString(runID) || agentRunIDPattern.MatchString(runID))
}

func validRelayEventID(eventID string) bool {
	return len(eventID) <= maxRelayIDBytes && eventIDPattern.MatchString(eventID)
}

func validateRunSummary(summary RunSummary) error {
	if _, ok := executionStatuses[summary.Status]; !ok {
		return errors.New("observability summary status is invalid")
	}
	if summary.DurationMs != nil && *summary.DurationMs < 0 {
		return errors.New("observability summary duration is invalid")
	}
	seen := make(map[string]struct{}, len(summary.ErrorFingerprints))
	for _, fingerprint := range summary.ErrorFingerprints {
		if !hashPattern.MatchString(fingerprint) {
			return errors.New("observability summary fingerprint is invalid")
		}
		if _, exists := seen[fingerprint]; exists {
			return errors.New("observability summary fingerprints must be unique")
		}
		seen[fingerprint] = struct{}{}
	}
	if err := summary.Correlation.validate(); err != nil {
		return fmt.Errorf("validate observability summary: %w", err)
	}
	switch {
	case workflowRunIDPattern.MatchString(summary.RunID) && summary.Correlation.WorkflowRunID != summary.RunID:
		return errors.New("observability summary workflow identity mismatch")
	case agentRunIDPattern.MatchString(summary.RunID) && summary.Correlation.AgentRunID != summary.RunID:
		return errors.New("observability summary agent identity mismatch")
	}
	if summary.UpdatedAt.IsZero() {
		return errors.New("observability summary timestamp is required")
	}
	return nil
}
