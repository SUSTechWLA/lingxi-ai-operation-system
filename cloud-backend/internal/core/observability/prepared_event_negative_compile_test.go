package observability

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

type preparedOwnerSink struct {
	mu     sync.Mutex
	owners []string
	events []Event
}

func (s *preparedOwnerSink) Write(ctx context.Context, event Event) error {
	owner, _ := trustedcontext.UserID(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.owners = append(s.owners, owner)
	s.events = append(s.events, event)
	return nil
}

func (*preparedOwnerSink) Close(context.Context) error { return nil }

func TestRawEventCannotCompileAsPreparedReplayCapability(t *testing.T) {
	command := exec.Command("go", "test", "./testdata/raw_prepared_replay")
	command.Dir = "."
	command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "go-cache"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("raw Event unexpectedly compiled as PreparedEvent: %s", output)
	}
	if !strings.Contains(string(output), "does not implement observability.PreparedEvent") {
		t.Fatalf("compile error did not enforce opaque capability:\n%s", output)
	}
}

func TestPublicPackageDecoderCannotMintPreparedCapability(t *testing.T) {
	command := exec.Command("go", "test", "./testdata/public_decoder_mint")
	command.Dir = "."
	command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "go-cache"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("public package decoder still mints PreparedEvent: %s", output)
	}
	if !strings.Contains(string(output), "undefined: observability.DecodePreparedEvent") {
		t.Fatalf("compile error did not prove decoder removal:\n%s", output)
	}
}
