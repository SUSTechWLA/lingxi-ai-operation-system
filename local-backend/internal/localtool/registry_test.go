package localtool

import (
	"context"
	"testing"
)

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

func TestIsAllowedCommandWithNormalization(t *testing.T) {
	if !IsAllowedCommand("local_file_import") {
		t.Error("lowercase local_file_import should be allowed after normalization")
	}
	if !IsAllowedCommand("  bundle_extract  ") {
		t.Error("whitespace-padded bundle_extract should be allowed after normalization")
	}
	if IsAllowedCommand("") {
		t.Error("empty string should not be allowed")
	}
	if IsAllowedCommand("  bash  ") {
		t.Error("whitespace-padded bash should not become allowed")
	}
}

func TestRegistryRejectsArbitraryShellCommands(t *testing.T) {
	reg := NewRegistry()
	reg.Register(ExecutorFunc(func(context.Context, Job) (*Result, error) {
		return &Result{Output: map[string]interface{}{"ok": true}}, nil
	}), "BUNDLE_EXTRACT")

	if !reg.CanExecute("BUNDLE_EXTRACT") {
		t.Fatal("registered semantic command should be executable")
	}
	for _, cmd := range []string{"bash", "sh", "cmd.exe", "powershell", "python arbitrary", "node arbitrary", "curl arbitrary", "rm", "mv arbitrary", "cp arbitrary", ""} {
		if reg.CanExecute(cmd) {
			t.Fatalf("%q must not be executable", cmd)
		}
	}
}

func TestRegistryDispatchesOnlyRegisteredWhitelistCommand(t *testing.T) {
	reg := NewRegistry()
	reg.Register(ExecutorFunc(func(_ context.Context, job Job) (*Result, error) {
		return &Result{Output: map[string]interface{}{"command": job.Command}}, nil
	}), "BUNDLE_EXTRACT")

	result, err := reg.Execute(context.Background(), Job{ID: "job-1", Command: "BUNDLE_EXTRACT"})
	if err != nil {
		t.Fatalf("execute registered command: %v", err)
	}
	if result.Output["command"] != "BUNDLE_EXTRACT" {
		t.Fatalf("unexpected output: %#v", result.Output)
	}

	if _, err := reg.Execute(context.Background(), Job{ID: "job-2", Command: "LOCAL_FILE_IMPORT"}); err == nil {
		t.Fatal("unregistered semantic command should fail closed")
	}
}
