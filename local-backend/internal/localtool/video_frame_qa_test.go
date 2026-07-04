package localtool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVideoFrameQAExecutorRejectsUnsafeProjectID(t *testing.T) {
	_, err := NewVideoFrameQAExecutor(t.TempDir()).Execute(t.Context(), Job{
		ProjectID: "../bad",
		Payload: map[string]interface{}{
			"input": "local://projects/test/renders/final.mp4",
		},
	})
	if err == nil {
		t.Fatal("expected unsafe project id error")
	}
}

func TestVideoFrameQAExecutorDelegatesAnalysisToStandardMCPTool(t *testing.T) {
	dataDir := t.TempDir()
	projectID := "vp_mcp"
	videoPath := filepath.Join(dataDir, "projects", projectID, "renders", "final.mp4")
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("mkdir video dir: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("not a real video; fake mcp does not inspect it"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	fake := &fakeVideoQAMCPClient{output: map[string]interface{}{
		"success":           true,
		"passed":            true,
		"score":             91,
		"reportRef":         "local://projects/vp_mcp/reports/video_frame_qa/video_frame_qa.json",
		"shotReports":       []interface{}{map[string]interface{}{"shotId": "SHOT_01", "decision": "PASS"}},
		"shotSpecLints":     []interface{}{},
		"shotSummaries":     []interface{}{map[string]interface{}{"shotId": "SHOT_01", "decision": "PASS"}},
		"repairPlan":        map[string]interface{}{"nextAction": "approve"},
		"needsRegeneration": false,
		"artifacts":         []interface{}{},
	}}
	executor := NewVideoFrameQAExecutorWithMCPClient(dataDir, fake)

	result, err := executor.Execute(t.Context(), Job{
		ProjectID: projectID,
		Payload: map[string]interface{}{
			"input":             "local://projects/vp_mcp/renders/final.mp4",
			"sampleIntervalSec": 3.0,
			"profile": map[string]interface{}{
				"profileId": "cinematic_story",
			},
			"shotList": []interface{}{
				map[string]interface{}{"id": "SHOT_01", "durationSec": 6.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if fake.toolName != "video_qa.analyze_video" {
		t.Fatalf("toolName = %q, want video_qa.analyze_video", fake.toolName)
	}
	if got := fake.args["projectId"]; got != projectID {
		t.Fatalf("projectId arg = %#v, want %q", got, projectID)
	}
	if got := fake.args["videoPath"]; got != videoPath {
		t.Fatalf("videoPath arg = %#v, want %q", got, videoPath)
	}
	if got := fake.args["videoRef"]; got != "local://projects/vp_mcp/renders/final.mp4" {
		t.Fatalf("videoRef arg = %#v", got)
	}
	if got := fake.args["sampleIntervalSec"]; got != 3.0 {
		t.Fatalf("sampleIntervalSec arg = %#v, want 3", got)
	}
	if got := fake.args["profile"]; got != "cinematic_story" {
		t.Fatalf("profile arg = %#v, want cinematic_story", got)
	}
	if got := result.Output["score"]; got != 91 {
		t.Fatalf("result score = %#v, want 91", got)
	}
}

func TestVideoFrameQAExecutorRunsDefaultVideoQAMCPServer(t *testing.T) {
	if os.Getenv("RUN_VIDEO_QA_MCP_E2E") != "1" {
		t.Skip("set RUN_VIDEO_QA_MCP_E2E=1 to run the video QA MCP E2E test")
	}
	dataDir := t.TempDir()
	projectID := "vp_mcp_e2e"
	videoPath := filepath.Join(dataDir, "projects", projectID, "renders", "final.mp4")
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("mkdir video dir: %v", err)
	}
	if output, err := exec.Command(
		"ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "color=c=black:s=320x180:d=1:r=1",
		"-pix_fmt", "yuv420p",
		videoPath,
	).CombinedOutput(); err != nil {
		t.Fatalf("create test video: %v: %s", err, string(output))
	}

	result, err := NewVideoFrameQAExecutor(dataDir).Execute(t.Context(), Job{
		ProjectID:  projectID,
		TimeoutSec: 30,
		Payload: map[string]interface{}{
			"input":             "local://projects/vp_mcp_e2e/renders/final.mp4",
			"sampleIntervalSec": 1.0,
			"shotList": []interface{}{
				map[string]interface{}{"id": "SHOT_01", "durationSec": 1.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Output["success"] != true {
		t.Fatalf("success = %#v, output=%#v", result.Output["success"], result.Output)
	}
	if result.Output["shotReports"] == nil {
		t.Fatalf("shotReports missing from output: %#v", result.Output)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "projects", projectID, "reports", "video_frame_qa", "shot_qa_reports.json")); err != nil {
		t.Fatalf("shot qa report not written: %v", err)
	}
}

type fakeVideoQAMCPClient struct {
	toolName string
	args     map[string]interface{}
	output   map[string]interface{}
}

func (f *fakeVideoQAMCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
	f.toolName = name
	f.args = args
	return f.output, nil
}

func (f *fakeVideoQAMCPClient) Close() error { return nil }
