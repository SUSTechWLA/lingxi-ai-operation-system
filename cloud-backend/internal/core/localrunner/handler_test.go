package localrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
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

func TestHandlerHeartbeatPreservesCapabilitiesNilAndExplicitEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{}
	router := gin.New()
	NewHandler(service, nil).RegisterRoutes(router)

	for _, body := range []string{
		`{"sessionId":"session_001","status":"online"}`,
		`{"sessionId":"session_001","status":"online","capabilities":[]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/local-runners/runner_001/heartbeat", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
	if len(service.heartbeats) != 2 || service.heartbeats[0].Capabilities != nil {
		t.Fatalf("omitted capabilities must remain nil: %#v", service.heartbeats)
	}
	if service.heartbeats[1].Capabilities == nil || len(*service.heartbeats[1].Capabilities) != 0 {
		t.Fatalf("explicit empty capabilities must remain a clear operation: %#v", service.heartbeats)
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

func TestHandlerRejectsInvalidCanonicalLocalOutputBeforeCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{
		ID: "local_job_contract", NodeID: "node_contract", ToolName: "native_contract", Status: JobRunning,
	}}
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name: "native_contract", Boundary: tool.BoundaryLocalNative,
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"assetId"}, "additionalProperties": false,
		},
	})
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).WithToolManifestResolver(registry).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_contract/complete", bytes.NewBufferString(`{
		"success":true,"output":{"unexpected":true}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_001")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
	if service.completeJobID != "" {
		t.Fatalf("invalid output was completed: %s", service.completeJobID)
	}
	if service.failJobID != "local_job_contract" || !strings.Contains(sink.failureError, "OUTPUT_SCHEMA_INVALID") {
		t.Fatalf("invalid output was not failed through node sink: service=%#v sink=%#v", service, sink)
	}
	if sink.successNodeID != "" {
		t.Fatalf("invalid output reported success: %#v", sink)
	}
}

func TestHandlerValidatesMCPStructuredContentInsteadOfWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{
		ID: "local_job_mcp", NodeID: "node_mcp", ToolName: "mcp_asset", MCPLogicalToolName: "mcp_asset", Status: JobRunning,
	}}
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name: "mcp_asset", Boundary: tool.BoundaryMCPProvider,
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"assetId"}, "additionalProperties": false,
		},
	})
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).WithToolManifestResolver(registry).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_mcp/complete", bytes.NewBufferString(`{
		"success":true,
		"output":{"content":[{"type":"text","text":"ok"}],"structuredContent":{"assetId":"asset-1"},"isError":false}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_001")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || service.completeJobID != "local_job_mcp" || sink.successNodeID != "node_mcp" {
		t.Fatalf("valid MCP completion failed: status=%d body=%s service=%#v sink=%#v", rec.Code, rec.Body.String(), service, sink)
	}
}

func TestHandlerTerminalCompletionReportDoesNotReenterOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{
		ID: "local_job_failed", NodeID: "node_failed", ToolName: "native_contract", Status: JobFailed,
		ResultCallbackState: CallbackDelivered, FollowupCallbackState: CallbackDelivered,
	}}
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_failed/complete", bytes.NewBufferString(`{"success":true,"output":{"result":"late"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_001")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("terminal retry status = %d body=%s", rec.Code, rec.Body.String())
	}
	if service.completeJobID != "" || sink.successNodeID != "" {
		t.Fatalf("FAILED completion reentered success: service=%#v sink=%#v", service, sink)
	}
}

func TestHandlerLateCompletionReplaysPendingFailureWithoutFlippingStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{
		ID: "local_job_failed_pending", NodeID: "node_failed_pending", Status: JobFailed,
		ErrorMessage: "durable failure", ResultCallbackState: CallbackPending, FollowupCallbackState: CallbackPending,
	}}
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_failed_pending/complete", bytes.NewBufferString(`{"success":true,"output":{"result":"late"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_001")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK || service.completeJobID != "" || service.job.Status != JobFailed {
		t.Fatalf("late completion mutated failure: status=%d service=%#v", res.Code, service)
	}
	if sink.successCalls != 0 || sink.failureCalls != 1 || sink.failureError != "durable failure" {
		t.Fatalf("late completion did not replay failure callback: %#v", sink)
	}
}

func TestHandlerReplaysPendingCompletedCallbackAndDeliveredRetryIsNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{ID: "local_job_replay", NodeID: "node_replay", Status: JobRunning}}
	sink := &fakeNodeResultSink{successErrors: []error{errors.New("temporary sink failure")}}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_replay/complete", bytes.NewBufferString(`{"success":true,"output":{"assetId":"asset-1"}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Runner-ID", "runner_001")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := request(); res.Code != http.StatusInternalServerError {
		t.Fatalf("first callback failure status=%d body=%s", res.Code, res.Body.String())
	}
	if service.job.Status != JobCompleted || service.job.ResultCallbackState != CallbackPending {
		t.Fatalf("terminal result was not persisted pending callback: %#v", service.job)
	}
	if res := request(); res.Code != http.StatusOK {
		t.Fatalf("pending callback replay status=%d body=%s", res.Code, res.Body.String())
	}
	if sink.successCalls != 2 || service.job.ResultCallbackState != CallbackDelivered || service.job.FollowupCallbackState != CallbackDelivered {
		t.Fatalf("pending callback not delivered exactly on retry: calls=%d job=%#v", sink.successCalls, service.job)
	}
	if res := request(); res.Code != http.StatusOK || sink.successCalls != 2 {
		t.Fatalf("delivered retry was not no-op: status=%d calls=%d", res.Code, sink.successCalls)
	}
}

func TestHandlerReplaysOnlyPendingArtifactCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{ID: "local_job_artifact", NodeID: "node_artifact", Status: JobRunning}}
	sink := &fakeNodeResultSink{}
	artifactCalls := 0
	router := gin.New()
	NewHandler(service, sink).WithArtifactSyncCallback(func(_ context.Context, _ *LocalJob, output map[string]interface{}) error {
		artifactCalls++
		if output["assetId"] != "asset-2" {
			t.Fatalf("artifact replay lost persisted output: %#v", output)
		}
		if artifactCalls == 1 {
			return errors.New("artifact index unavailable")
		}
		return nil
	}).RegisterRoutes(router)

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_artifact/complete", bytes.NewBufferString(`{"success":true,"output":{"assetId":"asset-2"}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Runner-ID", "runner_001")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := request(); res.Code != http.StatusInternalServerError {
		t.Fatalf("first artifact callback status=%d body=%s", res.Code, res.Body.String())
	}
	if res := request(); res.Code != http.StatusOK {
		t.Fatalf("artifact callback replay status=%d body=%s", res.Code, res.Body.String())
	}
	if sink.successCalls != 1 || artifactCalls != 2 {
		t.Fatalf("successful sink was replayed with artifact: sink=%d artifact=%d", sink.successCalls, artifactCalls)
	}
}

func TestHandlerReplaysPendingFailureCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{ID: "local_job_failure", NodeID: "node_failure", Status: JobRunning}}
	sink := &fakeNodeResultSink{failureErrors: []error{errors.New("temporary failure sink error")}}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_failure/fail", bytes.NewBufferString(`{"success":false,"error":{"code":"BROKEN","message":"preserved failure"},"retryable":true}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Runner-ID", "runner_001")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := request(); res.Code != http.StatusInternalServerError {
		t.Fatalf("first failure callback status=%d body=%s", res.Code, res.Body.String())
	}
	if res := request(); res.Code != http.StatusOK {
		t.Fatalf("failure callback replay status=%d body=%s", res.Code, res.Body.String())
	}
	if sink.failureCalls != 2 || sink.failureError != "preserved failure" || service.job.ResultCallbackState != CallbackDelivered {
		t.Fatalf("failure replay lost state/content: sink=%#v job=%#v", sink, service.job)
	}
}

func TestHandlerTreatsMCPIsErrorAsFailureWithoutOutputSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{ID: "local_job_mcp_error", NodeID: "node_mcp_error", ToolName: "mcp_error", Status: JobRunning}}
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{Name: "mcp_error", Boundary: tool.BoundaryMCPProvider})
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).WithToolManifestResolver(registry).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_mcp_error/complete", bytes.NewBufferString(`{
		"success":true,"output":{"isError":true,"content":[{"type":"text","text":"remote renderer failed"}]}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_001")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity || service.completeJobID != "" || service.failReq.Error["code"] != "MCP_TOOL_ERROR" {
		t.Fatalf("MCP isError was not failed: status=%d body=%s service=%#v", res.Code, res.Body.String(), service)
	}
	if !strings.Contains(sink.failureError, "remote renderer failed") {
		t.Fatalf("MCP content was not preserved for failure sink: %#v", sink)
	}
}

func TestHandlerCompleteHyperFramesRenderNormalizesVideoArtifact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{
		job: &LocalJob{
			ID:        "local_job_001",
			ProjectID: "project_001",
			TaskID:    "task_001",
			NodeID:    "render_exec",
			ToolName:  "hyperframes_renderer",
			Command:   CommandHyperFramesRender,
			Payload: map[string]interface{}{
				"outputName": "final.mp4",
			},
		},
	}
	sink := &fakeNodeResultSink{}
	router := gin.New()
	NewHandler(service, sink).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_001/complete", bytes.NewBufferString(`{
		"success": true,
		"output": {"renderTimeMs":12345,"fps":30,"width":1920,"height":1080,"sizeBytes":123456}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_001")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	artifacts, ok := service.completeReq.Output["artifacts"].([]interface{})
	if !ok || len(artifacts) != 1 {
		t.Fatalf("expected one normalized artifact, got %#v", service.completeReq.Output["artifacts"])
	}
	video, ok := artifacts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("artifact should be object, got %#v", artifacts[0])
	}
	assertVideoArtifactContract(t, video)
	if sink.successOutput["artifacts"] == nil {
		t.Fatalf("node success output should receive normalized artifacts: %#v", sink.successOutput)
	}
}

func assertVideoArtifactContract(t *testing.T, video map[string]interface{}) {
	t.Helper()
	expected := map[string]interface{}{
		"unitId":         "final-video",
		"kind":           "VIDEO",
		"name":           "final.mp4",
		"storageType":    "local",
		"storageRef":     "local://projects/project_001/renders/final.mp4",
		"mimeType":       "video/mp4",
		"sizeBytes":      float64(123456),
		"status":         "valid",
		"humanApproved":  false,
		"producedByTool": "hyperframes_renderer",
		"producedByRole": "渲染制片",
	}
	for key, want := range expected {
		if got := video[key]; got != want {
			t.Fatalf("video[%s] = %#v, want %#v; artifact=%#v", key, got, want, video)
		}
	}
	dependsOn, ok := video["dependsOn"].([]interface{})
	if !ok || len(dependsOn) != 2 || dependsOn[0] != "PREVIEW_SNAPSHOTS" || dependsOn[1] != "HYPERFRAMES_PROJECT" {
		t.Fatalf("unexpected dependsOn: %#v", video["dependsOn"])
	}
	metadata, ok := video["metadata"].(map[string]interface{})
	if !ok || metadata["renderTimeMs"] != float64(12345) || metadata["fps"] != float64(30) {
		t.Fatalf("unexpected metadata: %#v", video["metadata"])
	}
	if metadata["width"] != float64(1920) || metadata["height"] != float64(1080) {
		t.Fatalf("metadata should include output dimensions: %#v", video["metadata"])
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

func TestHandlerRejectsAtomicMutationAuthorizationFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{mutationErr: ErrJobAccessDenied}
	router := gin.New()
	NewHandler(service, nil).RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/local_job_stale/progress", bytes.NewBufferString(`{"progress":0.5}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner_old")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

type fakeRunnerService struct {
	registerReq   RegisterRunnerRequest
	claimRunnerID string
	completeJobID string
	completeReq   CompleteJobRequest
	failJobID     string
	failReq       FailJobRequest
	job           *LocalJob
	heartbeats    []HeartbeatRequest
	mutationErr   error
	markErr       error
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

func (f *fakeRunnerService) Heartbeat(_ context.Context, _ string, req HeartbeatRequest) error {
	f.heartbeats = append(f.heartbeats, req)
	return nil
}

func (f *fakeRunnerService) ClaimJob(_ context.Context, runnerID string) (*LocalJob, error) {
	f.claimRunnerID = runnerID
	return f.job, nil
}

func (f *fakeRunnerService) ReportProgress(_ context.Context, _ JobMutationIdentity, _ string, _ ProgressRequest) error {
	return f.mutationErr
}

func (f *fakeRunnerService) CompleteJob(_ context.Context, _ JobMutationIdentity, jobID string, req CompleteJobRequest) (*LocalJob, error) {
	f.completeJobID = jobID
	f.completeReq = req
	if f.mutationErr == nil && f.job != nil {
		f.job.Status = JobCompleted
		f.job.Output = req.Output
		f.job.ResultCallbackState = CallbackPending
		f.job.FollowupCallbackState = CallbackPending
	}
	return f.job, f.mutationErr
}

func (f *fakeRunnerService) FailJob(_ context.Context, _ JobMutationIdentity, jobID string, req FailJobRequest) (*LocalJob, error) {
	f.failJobID = jobID
	f.failReq = req
	if f.mutationErr == nil && f.job != nil {
		f.job.Status = JobFailed
		f.job.Error = req.Error
		f.job.ErrorMessage = errorMessageFromMap(req.Error)
		f.job.ResultCallbackState = CallbackPending
		f.job.FollowupCallbackState = CallbackPending
	}
	return f.job, f.mutationErr
}

func (f *fakeRunnerService) MarkCallbackDelivered(_ context.Context, _ JobMutationIdentity, _ string, phase CallbackPhase) error {
	if f.markErr != nil || f.job == nil {
		return f.markErr
	}
	switch phase {
	case CallbackPhaseResult:
		f.job.ResultCallbackState = CallbackDelivered
	case CallbackPhaseFollowup:
		f.job.FollowupCallbackState = CallbackDelivered
	}
	return nil
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
	successNodeID  string
	successOutput  map[string]interface{}
	failureNodeID  string
	failureError   string
	progressNodeID string
	progressValue  float64
	progressStep   string
	progressMsg    string
	successCalls   int
	failureCalls   int
	successErrors  []error
	failureErrors  []error
}

func (f *fakeNodeResultSink) OnSuccess(_ context.Context, nodeID string, output map[string]interface{}) error {
	f.successCalls++
	f.successNodeID = nodeID
	f.successOutput = output
	if len(f.successErrors) > 0 {
		err := f.successErrors[0]
		f.successErrors = f.successErrors[1:]
		return err
	}
	return nil
}

func (f *fakeNodeResultSink) OnFailure(_ context.Context, nodeID string, errorMessage string) error {
	f.failureCalls++
	f.failureNodeID = nodeID
	f.failureError = errorMessage
	if len(f.failureErrors) > 0 {
		err := f.failureErrors[0]
		f.failureErrors = f.failureErrors[1:]
		return err
	}
	return nil
}

func (f *fakeNodeResultSink) OnProgress(_ context.Context, nodeID string, progress float64, step, message string) error {
	f.progressNodeID = nodeID
	f.progressValue = progress
	f.progressStep = step
	f.progressMsg = message
	return nil
}
