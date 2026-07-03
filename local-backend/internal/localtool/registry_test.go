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
	if !IsAllowedCommand("hyperframes_render") {
		t.Error("lowercase hyperframes_render should be allowed after normalization")
	}
	if !IsAllowedCommand("  ffmpeg_probe  ") {
		t.Error("whitespace-padded ffmpeg_probe should be allowed after normalization")
	}
	if !IsAllowedCommand("video_frame_qa") {
		t.Error("lowercase video_frame_qa should be allowed after normalization")
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
	}), "FFMPEG_PROBE")

	if !reg.CanExecute("FFMPEG_PROBE") {
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
	}), "ARTIFACT_PACKAGE")

	result, err := reg.Execute(context.Background(), Job{ID: "job-1", Command: "ARTIFACT_PACKAGE"})
	if err != nil {
		t.Fatalf("execute registered command: %v", err)
	}
	if result.Output["command"] != "ARTIFACT_PACKAGE" {
		t.Fatalf("unexpected output: %#v", result.Output)
	}

	if _, err := reg.Execute(context.Background(), Job{ID: "job-2", Command: "FFMPEG_PROBE"}); err == nil {
		t.Fatal("unregistered semantic command should fail closed")
	}
}
