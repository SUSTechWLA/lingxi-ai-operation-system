package localrunner

import "testing"

func TestIsValidCommand(t *testing.T) {
	valid := []string{
		"HYPERFRAMES_PROJECT_GENERATE",
		"HYPERFRAMES_RENDER",
		"HYPERFRAMES_LINT",
		"HYPERFRAMES_SNAPSHOT",
		"HYPERGEN_RENDER",
		"FFMPEG_PROBE",
		"FFMPEG_CLIP_EXTRACT",
		"FFMPEG_ASSEMBLE",
		"AUDIO_EXTRACT",
		"AUDIO_NORMALIZE",
		"ASR_TRANSCRIBE",
		"ARTIFACT_PACKAGE",
		"LOCAL_FILE_IMPORT",
		"LOCAL_MEDIA_INDEX",
		"LOCAL_MCP_TOOL_CALL",
		"BUNDLE_EXTRACT",
	}
	for _, cmd := range valid {
		if !IsValidCommand(cmd) {
			t.Fatalf("%s should be valid", cmd)
		}
	}

	invalid := []string{"rm -rf /", "bash", "sh", "cmd", "powershell", "python", "node", "curl", ""}
	for _, cmd := range invalid {
		if IsValidCommand(cmd) {
			t.Fatalf("%q should NOT be valid", cmd)
		}
	}
}

func TestLocalJobModel(t *testing.T) {
	job := &LocalJob{
		ID:        "job-1",
		ProjectID: "proj-1",
		NodeID:    "node-1",
		ToolName:  "hyperframes_renderer",
		Command:   "HYPERFRAMES_RENDER",
		Payload: map[string]interface{}{
			"projectDir": "local://projects/proj-1/hyperframes",
		},
		Status:   JobPending,
		Progress: 0.0,
		ArtifactPolicy: LocalArtifactPolicy{
			Location:            "local",
			SyncMetadataToCloud: true,
			SyncFileToCloud:     false,
		},
	}
	if !IsValidCommand(job.Command) {
		t.Error("job command should be valid")
	}
	if job.NodeID == "" || job.ToolName == "" {
		t.Fatalf("local jobs must keep orchestration identity: %#v", job)
	}
	if job.Status != JobPending {
		t.Errorf("expected PENDING, got %s", job.Status)
	}
	if job.ArtifactPolicy.SyncFileToCloud {
		t.Fatal("large local artifacts should default to metadata-only cloud sync")
	}
}

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HYPERFRAMES_RENDER", "HYPERFRAMES_RENDER"},
		{"hyperframes_render", "HYPERFRAMES_RENDER"},
		{"HyperFrames_Render", "HYPERFRAMES_RENDER"},
		{"  FFMPEG_PROBE  ", "FFMPEG_PROBE"},
		{"\tARTIFACT_PACKAGE\n", "ARTIFACT_PACKAGE"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		result := NormalizeCommand(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeCommand(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsValidCommandWithNormalization(t *testing.T) {
	// Verify that IsValidCommand normalizes input before checking
	if !IsValidCommand("hyperframes_render") {
		t.Error("lowercase hyperframes_render should be valid after normalization")
	}
	if !IsValidCommand("  ffmpeg_probe  ") {
		t.Error("whitespace-padded ffmpeg_probe should be valid after normalization")
	}
	if IsValidCommand("") {
		t.Error("empty string should not be valid")
	}
	if IsValidCommand("  bash  ") {
		t.Error("whitespace-padded bash should not become valid")
	}
}

func TestLocalRunnerModel(t *testing.T) {
	runner := &LocalRunner{
		ID:            "runner-1",
		DeviceID:      "device-macbook-1",
		UserID:        "user-1",
		RunnerVersion: "1.0.0",
		Platform: PlatformInfo{
			OS:       "darwin",
			Arch:     "arm64",
			Hostname: "MacBook Pro",
		},
		WorkspaceRoot: "local://aios/projects",
		Capabilities: []RunnerCapability{
			{ToolName: "hyperframes_renderer", Command: "HYPERFRAMES_RENDER", Available: true, Version: "adapter 1.0.0"},
		},
		Status: RunnerOnline,
	}
	if runner.Status != RunnerOnline {
		t.Errorf("expected ONLINE, got %s", runner.Status)
	}
	if runner.DeviceID == "" || len(runner.Capabilities) != 1 {
		t.Fatalf("runner should retain device identity and capability probe result: %#v", runner)
	}
}

func TestJobStatusLifecycle(t *testing.T) {
	// Verify all lifecycle states are defined and distinct.
	states := []JobStatus{JobPending, JobClaimed, JobRunning, JobCompleted, JobFailed}
	seen := make(map[JobStatus]bool)
	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate JobStatus: %s", s)
		}
		seen[s] = true
		if s == "" {
			t.Error("empty JobStatus not allowed")
		}
	}
}

func TestRunnerStatusConstants(t *testing.T) {
	if string(RunnerOnline) != "ONLINE" {
		t.Error("RunnerOnline mismatch")
	}
	if string(RunnerOffline) != "OFFLINE" {
		t.Error("RunnerOffline mismatch")
	}
	if string(RunnerRevoked) != "REVOKED" {
		t.Error("RunnerRevoked mismatch")
	}
	// Verify all statuses are distinct.
	if RunnerOnline == RunnerOffline || RunnerOnline == RunnerRevoked || RunnerOffline == RunnerRevoked {
		t.Error("runner statuses must be distinct")
	}
}
