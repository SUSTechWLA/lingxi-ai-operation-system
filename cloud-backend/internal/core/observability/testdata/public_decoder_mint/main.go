package publicdecodermint

import "github.com/tangying-ai/aios-core/internal/core/observability"

func mint(payload []byte, binding observability.PreparedEventBinding) (observability.PreparedEvent, error) {
	return observability.DecodePreparedEvent(payload, binding)
}
