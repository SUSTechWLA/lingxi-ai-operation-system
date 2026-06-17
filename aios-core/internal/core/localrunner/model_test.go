package localrunner

import "testing"

func TestIsValidCommand(t *testing.T) {
	if !IsValidCommand("HYPERGEN_RENDER") {
		t.Error("HYPERGEN_RENDER should be valid")
	}
	if !IsValidCommand("FFMPEG_PROBE") {
		t.Error("FFMPEG_PROBE should be valid")
	}
	if IsValidCommand("rm -rf /") {
		t.Error("arbitrary shell commands should NOT be valid")
	}
	if IsValidCommand("") {
		t.Error("empty command should not be valid")
	}
}

func TestValidCommandsCount(t *testing.T) {
	if len(ValidCommands) < 4 {
		t.Errorf("expected at least 4 commands, got %d", len(ValidCommands))
	}
}

func TestLocalJobModel(t *testing.T) {
	job := &LocalJob{
		ID:        "job-1",
		ProjectID: "proj-1",
		Command:   "HYPERGEN_RENDER",
		Status:    JobPending,
		Progress:  0.0,
	}
	if !IsValidCommand(job.Command) {
		t.Error("job command should be valid")
	}
	if job.Status != JobPending {
		t.Errorf("expected PENDING, got %s", job.Status)
	}
}

func TestLocalRunnerModel(t *testing.T) {
	runner := &LocalRunner{
		ID:     "runner-1",
		Name:   "MacBook Pro",
		Status: RunnerOnline,
	}
	if runner.Status != RunnerOnline {
		t.Errorf("expected ONLINE, got %s", runner.Status)
	}
}
