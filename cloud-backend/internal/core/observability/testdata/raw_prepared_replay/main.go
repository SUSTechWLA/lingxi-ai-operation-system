package rawpreparedreplay

import (
	"context"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

func replayRawEvent(emitter observability.PersistentPreparedEventEmitter) error {
	return emitter.ReplayPreparedAndWait(context.Background(), observability.Event{})
}
