package jimengmcp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls []fakeCall
	out   CommandOutput
	err   error
}

type fakeCall struct {
	name string
	args []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (CommandOutput, error) {
	r.calls = append(r.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	return r.out, r.err
}

func TestAdapterGenerateImageBuildsText2ImageCommand(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: `{"submit_id":"img-123","gen_status":"querying"}`}}
	adapter := NewAdapter(AdapterConfig{Runner: runner})

	result, err := adapter.GenerateImage(context.Background(), GenerateImageRequest{
		Prompt:         "cinematic skyline",
		Ratio:          "16:9",
		ResolutionType: "2k",
		PollSeconds:    30,
		GenerateNum:    2,
	})
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if result.SubmitID != "img-123" {
		t.Fatalf("SubmitID = %q, want img-123", result.SubmitID)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "dreamina" {
		t.Fatalf("command = %q, want dreamina", call.name)
	}
	want := []string{"text2image", "--prompt=cinematic skyline", "--ratio=16:9", "--resolution_type=2k", "--generate_num=2", "--poll=30"}
	if strings.Join(call.args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("args = %#v, want %#v", call.args, want)
	}
}

func TestAdapterGenerateVideoBuildsImage2VideoCommand(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: `{"submit_id":"vid-456","gen_status":"success","downloaded_files":["/tmp/out.mp4"]}`}}
	adapter := NewAdapter(AdapterConfig{Runner: runner})

	result, err := adapter.GenerateVideo(context.Background(), GenerateVideoRequest{
		Mode:            "image2video",
		Prompt:          "slow dolly in",
		Image:           "/tmp/frame.png",
		Duration:        5,
		VideoResolution: "720p",
		PollSeconds:     30,
	})
	if err != nil {
		t.Fatalf("GenerateVideo returned error: %v", err)
	}
	if result.GenStatus != "success" {
		t.Fatalf("GenStatus = %q, want success", result.GenStatus)
	}
	if len(result.DownloadedFiles) != 1 || result.DownloadedFiles[0] != "/tmp/out.mp4" {
		t.Fatalf("DownloadedFiles = %#v", result.DownloadedFiles)
	}
	want := []string{"image2video", "--image=/tmp/frame.png", "--prompt=slow dolly in", "--duration=5", "--video_resolution=720p", "--poll=30"}
	if strings.Join(runner.calls[0].args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("args = %#v, want %#v", runner.calls[0].args, want)
	}
}

func TestAdapterRejectsGenerationWithoutSubmitID(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: `{"gen_status":"querying"}`}}
	adapter := NewAdapter(AdapterConfig{Runner: runner})

	if _, err := adapter.GenerateImage(context.Background(), GenerateImageRequest{Prompt: "x"}); err == nil {
		t.Fatal("GenerateImage error = nil, want missing submit_id error")
	}
}

func TestAdapterRejectsFailedGenerationStatus(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: `{"submit_id":"bad-1","gen_status":"fail","fail_reason":"quota exhausted"}`}}
	adapter := NewAdapter(AdapterConfig{Runner: runner})

	_, err := adapter.GenerateImage(context.Background(), GenerateImageRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("GenerateImage error = nil, want fail status error")
	}
	if !strings.Contains(err.Error(), "quota exhausted") {
		t.Fatalf("error = %q, want fail reason", err.Error())
	}
}

func TestAdapterReturnsCommandErrorWithStderr(t *testing.T) {
	runner := &fakeRunner{
		out: CommandOutput{Stderr: "not logged in"},
		err: errors.New("exit status 1"),
	}
	adapter := NewAdapter(AdapterConfig{Runner: runner})

	_, err := adapter.GenerateImage(context.Background(), GenerateImageRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("GenerateImage error = nil, want command error")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("error = %q, want stderr", err.Error())
	}
}

func TestAdapterLoginHeadlessParsesKeyValueOutput(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: "verification_uri: https://example.com/device\nuser_code: ABCD-EFGH\ndevice_code: device-1\n"}}
	adapter := NewAdapter(AdapterConfig{Runner: runner})

	result, err := adapter.LoginHeadless(context.Background())
	if err != nil {
		t.Fatalf("LoginHeadless returned error: %v", err)
	}
	if result["user_code"] != "ABCD-EFGH" {
		t.Fatalf("user_code = %#v", result["user_code"])
	}
	if result["device_code"] != "device-1" {
		t.Fatalf("device_code = %#v", result["device_code"])
	}
}
