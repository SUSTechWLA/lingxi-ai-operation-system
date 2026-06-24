package localrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestHandlerRegisterRunnerUsesEdgeRunProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{}
	router := gin.New()
	NewHandler(service, nil).RegisterRoutes(router)

	body := `{
		"deviceId":"device_macbook_001",
		"userId":"user_001",
		"runnerVersion":"1.0.0",
		"platform":{"os":"darwin","arch":"arm64","hostname":"Wang-MacBook"},
		"workspaceRoot":"local://aios/projects",
		"capabilities":[{"toolName":"hyperframes_renderer","command":"HYPERFRAMES_RENDER","available":true,"version":"adapter 1.0.0"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/local-runners/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if service.registerReq.DeviceID != "device_macbook_001" || service.registerReq.Capabilities[0].Command != "HYPERFRAMES_RENDER" {
		t.Fatalf("register request not decoded: %#v", service.registerReq)
	}

	var resp RegisterRunnerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RunnerID == "" || resp.SessionID == "" || resp.HeartbeatIntervalSec != 15 || resp.PollIntervalSec != 3 {
		t.Fatalf("unexpected register response: %#v", resp)
	}
}

func TestHandlerRegisterRunnerUsesAuthenticatedUserAndDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), "user_auth")
		ctx = auth.ContextWithDevice(ctx, "device_auth")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	NewHandler(service, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-runners/register", bytes.NewBufferString(`{
		"deviceId":"device_spoofed",
		"userId":"user_spoofed",
		"runnerVersion":"1.0.0",
		"platform":{"os":"darwin","arch":"arm64","hostname":"Wang-MacBook"},
		"workspaceRoot":"local://aios/projects",
		"capabilities":[{"toolName":"artifact_packager","command":"ARTIFACT_PACKAGE","available":true}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if service.registerReq.UserID != "user_auth" || service.registerReq.DeviceID != "device_auth" {
		t.Fatalf("register should use authenticated identity, got %#v", service.registerReq)
	}
}

func TestHandlerClaimJobReturnsSemanticLocalJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{
		job: &LocalJob{
			ID:         "local_job_001",
			ProjectID:  "project_001",
			NodeID:     "node_hyperframes_render",
			ToolName:   "hyperframes_renderer",
			Command:    "HYPERFRAMES_RENDER",
			Payload:    map[string]interface{}{"projectDir": "local://projects/project_001/hyperframes", "fps": float64(30)},
			TimeoutSec: 1800,
			ArtifactPolicy: LocalArtifactPolicy{
				Location:            "local",
				SyncMetadataToCloud: true,
				SyncFileToCloud:     false,
			},
		},
	}
	router := gin.New()
	NewHandler(service, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/local-runners/runner_001/jobs/claim", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if service.claimRunnerID != "runner_001" {
		t.Fatalf("claim runner id = %q", service.claimRunnerID)
	}

	var resp ClaimJobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Job == nil || resp.Job.NodeID != "node_hyperframes_render" || resp.Job.ToolName != "hyperframes_renderer" {
		t.Fatalf("unexpected claim response: %#v", resp)
	}
	if resp.Job.Payload["projectDir"] != "local://projects/project_001/hyperframes" {
		t.Fatalf("payload should be a JSON object, got %#v", resp.Job.Payload)
	}
}

func TestHandlerCompleteJobAdvancesNodeResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{
		job: &LocalJob{
			ID:     "local_job_001",
			NodeID: "node_hyperframes_render",
		},
	}
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_001/complete", bytes.NewBufferString(`{
		"success": true,
		"output": {"summary":"视频渲染完成","artifacts":[{"kind":"VIDEO","storageRef":"local://projects/project_001/renders/final.mp4"}]}
	}`))
	req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Runner-ID", "runner_001")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if service.completeJobID != "local_job_001" {
		t.Fatalf("complete job id = %q", service.completeJobID)
	}
	if sink.successNodeID != "node_hyperframes_render" || sink.successOutput["summary"] != "视频渲染完成" {
		t.Fatalf("node success not reported: %#v", sink)
	}
}

func TestHandlerFailJobAdvancesNodeFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{
		job: &LocalJob{
			ID:     "local_job_001",
			NodeID: "node_hyperframes_render",
		},
	}
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_001/fail", bytes.NewBufferString(`{
		"success": false,
		"error": {"code":"LOCAL_TOOL_EXEC_FAILED","message":"HyperFrames Render Service 不可用"},
		"retryable": true
	}`))
	req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Runner-ID", "runner_001")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if sink.failureNodeID != "node_hyperframes_render" || sink.failureError != "HyperFrames Render Service 不可用" {
		t.Fatalf("node failure not reported: %#v", sink)
	}
}

type fakeRunnerService struct {
	registerReq   RegisterRunnerRequest
	claimRunnerID string
	completeJobID string
	job           *LocalJob
}

func (f *fakeRunnerService) RegisterRunner(_ context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	f.registerReq = req
	return &RegisterRunnerResponse{
		RunnerID:             "runner_001",
		SessionID:            "runner_session_001",
		HeartbeatIntervalSec: 15,
		PollIntervalSec:      3,
	}, nil
}

func (f *fakeRunnerService) Heartbeat(_ context.Context, _ string, _ HeartbeatRequest) error {
	return nil
}

func (f *fakeRunnerService) ClaimJob(_ context.Context, runnerID string) (*LocalJob, error) {
	f.claimRunnerID = runnerID
	return f.job, nil
}

func (f *fakeRunnerService) ReportProgress(_ context.Context, _ string, _ ProgressRequest) error {
	return nil
}

func (f *fakeRunnerService) CompleteJob(_ context.Context, jobID string, _ CompleteJobRequest) (*LocalJob, error) {
	f.completeJobID = jobID
	return f.job, nil
}

func (f *fakeRunnerService) FailJob(_ context.Context, _ string, _ FailJobRequest) (*LocalJob, error) {
	return f.job, nil
}

func (f *fakeRunnerService) GetJob(_ context.Context, _ string) (*LocalJob, error) {
	return f.job, nil
}

func (f *fakeRunnerService) ValidateRunnerAccess(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (f *fakeRunnerService) ValidateJobAccess(_ context.Context, _, _, _ string) error {
	return nil
}

type fakeNodeResultSink struct {
	successNodeID string
	successOutput map[string]interface{}
	failureNodeID string
	failureError  string
}

func (f *fakeNodeResultSink) OnSuccess(_ context.Context, nodeID string, output map[string]interface{}) error {
	f.successNodeID = nodeID
	f.successOutput = output
	return nil
}

func (f *fakeNodeResultSink) OnFailure(_ context.Context, nodeID string, errorMessage string) error {
	f.failureNodeID = nodeID
	f.failureError = errorMessage
	return nil
}
