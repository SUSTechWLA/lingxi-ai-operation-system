package agentruntime

import (
	"strings"
	"testing"
)

func TestTerminalAckAndReleaseSQLRequireExactEventAndClaim(t *testing.T) {
	for name, query := range map[string]string{"ack": ackTerminalEventSQL, "release": releaseTerminalEventSQL} {
		for _, fragment := range []string{"id=$1", "terminal_event_id=$2", "terminal_event_claim_token=$3"} {
			if !strings.Contains(query, fragment) {
				t.Fatalf("%s query missing %q: %s", name, fragment, query)
			}
		}
	}
}
