package localrunner

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerShotRegenerationCompletionPreservesExplicitDurableMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{
		ID: "job-1", ProjectID: "vp-1", TaskID: "agent-task-1", NodeID: "node-1",
		Payload: map[string]interface{}{"shotRegenerationTaskId": "regen-task-1", "targetShotId": "shot-012", "shotRegenerationRunId": "run-1"},
	}}
	var synced map[string]interface{}
	var syncedJob *LocalJob
	router := gin.New()
	NewHandler(service, nil).WithArtifactSyncCallback(func(_ context.Context, job *LocalJob, output map[string]interface{}) error {
		syncedJob = job
		synced = output
		return nil
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/job-1/complete", bytes.NewBufferString(`{
		"success":true,
		"output":{"metadata":{"shotRegenerationTaskId":"regen-task-1","relatedShotId":"shot-012"},"artifacts":[{"kind":"VIDEO","metadata":{"candidateId":"candidate-2"}}]}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner-1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	metadata, _ := synced["metadata"].(map[string]interface{})
	if metadata["shotRegenerationTaskId"] != "regen-task-1" || metadata["relatedShotId"] != "shot-012" {
		t.Fatalf("durable regeneration metadata not preserved: %#v", synced)
	}
	if syncedJob == nil || syncedJob.Payload["shotRegenerationRunId"] != "run-1" {
		t.Fatalf("artifact callback did not receive durable job provenance: %+v", syncedJob)
	}
}

func TestHandlerShotRegenerationFailureInvokesCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunnerService{job: &LocalJob{
		ID: "job-1", ProjectID: "vp-1", TaskID: "agent-task-1", NodeID: "node-1",
		Payload: map[string]interface{}{"shotRegenerationTaskId": "regen-task-1", "targetShotId": "shot-012"},
	}}
	var callbackJob *LocalJob
	var callbackReason string
	router := gin.New()
	NewHandler(service, nil).WithJobFailureCallback(func(_ context.Context, job *LocalJob, reason string) error {
		callbackJob, callbackReason = job, reason
		return nil
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-jobs/job-1/fail", bytes.NewBufferString(`{
		"success":false,"error":{"message":"provider timeout"},"retryable":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-ID", "runner-1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if callbackJob == nil || callbackJob.ProjectID != "vp-1" || callbackReason != "provider timeout" {
		t.Fatalf("callback job=%+v reason=%q", callbackJob, callbackReason)
	}
}
