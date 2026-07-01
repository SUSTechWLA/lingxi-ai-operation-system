package localrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSendsAuthDeviceAndSessionHeaders(t *testing.T) {
	var seenAuth, seenDevice, seenSession string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenDevice = r.Header.Get("X-Device-ID")
		seenSession = r.Header.Get("X-Runner-Session-ID")
		if r.URL.Path != "/api/local-runners/runner_001/heartbeat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer server.Close()

	client := NewClient(Config{
		CloudAPIBase: server.URL,
		UserToken:    "token-123",
		DeviceID:     "device-123",
		SessionID:    "session-123",
	})
	if err := client.Heartbeat(context.Background(), "runner_001", HeartbeatRequest{Status: "online"}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if seenAuth != "Bearer token-123" || seenDevice != "device-123" || seenSession != "session-123" {
		t.Fatalf("headers auth=%q device=%q session=%q", seenAuth, seenDevice, seenSession)
	}
}

func TestClientClaimsNullJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/local-runners/runner_001/jobs/claim" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ClaimJobResponse{})
	}))
	defer server.Close()

	client := NewClient(Config{CloudAPIBase: server.URL})
	job, err := client.ClaimJob(context.Background(), "runner_001")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil job, got %#v", job)
	}
}

func TestClientAcceptsCloudAPIBaseWithAPISuffix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/local-runners/runner_001/jobs/claim" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ClaimJobResponse{})
	}))
	defer server.Close()

	client := NewClient(Config{CloudAPIBase: server.URL + "/api"})
	if _, err := client.ClaimJob(context.Background(), "runner_001"); err != nil {
		t.Fatalf("claim: %v", err)
	}
}
