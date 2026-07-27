package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTerminalAckAndReleaseSQLRequireExactEventAndClaim(t *testing.T) {
	for name, query := range map[string]string{
		"ack": ackTerminalEventSQL, "release": releaseTerminalEventSQL, "freeze": freezeTerminalEventSQL,
		"callback": markTerminalCallbackDeliveredSQL, "observability": markTerminalObservabilityDeliveredSQL,
	} {
		for _, fragment := range []string{"id=$1", "terminal_event_id=$2", "terminal_event_claim_token=$3"} {
			if !strings.Contains(query, fragment) {
				t.Fatalf("%s query missing %q: %s", name, fragment, query)
			}
		}
	}
	for _, fragment := range []string{
		"terminal_event_callback_delivered_at IS NOT NULL",
		"terminal_event_observability_delivered_at IS NOT NULL",
	} {
		if !strings.Contains(ackTerminalEventSQL, fragment) {
			t.Fatalf("ack query missing %q: %s", fragment, ackTerminalEventSQL)
		}
	}
}

func TestRepositoryFreezeTerminalEventWritesExactClaimedPayload(t *testing.T) {
	db := &fakeAgentRunDB{}
	delivery := TerminalEventDelivery{
		RunID: "run-1", EventID: "evt_terminal_1", ClaimToken: "claim-1",
		Event: RunTerminalEvent{
			EventID: "evt_terminal_1", CallbackIdempotencyKey: "evt_terminal_1", RunID: "run-1", Status: RunStatusSuccess,
		},
	}
	frozen, err := newRepositoryWithDB(db).FreezeTerminalEvent(context.Background(), delivery)
	if err != nil || !frozen || len(db.execs) != 1 {
		t.Fatalf("frozen=%v error=%v calls=%#v", frozen, err, db.execs)
	}
	if len(db.execs[0].args) != 4 || db.execs[0].args[0] != delivery.RunID ||
		db.execs[0].args[1] != delivery.EventID || db.execs[0].args[2] != delivery.ClaimToken {
		t.Fatalf("freeze arguments=%#v", db.execs[0].args)
	}
	payload, ok := db.execs[0].args[3].([]byte)
	if !ok {
		t.Fatalf("freeze payload type=%T", db.execs[0].args[3])
	}
	var decoded RunTerminalEvent
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.CallbackIdempotencyKey != delivery.EventID {
		t.Fatalf("freeze payload=%s error=%v", payload, err)
	}
}
