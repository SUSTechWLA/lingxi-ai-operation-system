package localtool

import (
	"context"
	"testing"
)

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
