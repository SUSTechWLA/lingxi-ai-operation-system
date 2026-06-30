package localagent

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthAndPathsUseLocalDataDir(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root, CloudAPIBase: "https://cloud.example.com/api"})

	req := httptest.NewRequest(http.MethodGet, "/api/local/health", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var health map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("invalid health response: %v", err)
	}
	if health["service"] != "tangying-local-agent" {
		t.Fatalf("unexpected service: %#v", health)
	}
	if health["cloudApiBase"] != "https://cloud.example.com/api" {
		t.Fatalf("cloud API base not exposed: %#v", health)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/paths", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paths status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var paths Paths
	if err := json.Unmarshal(rec.Body.Bytes(), &paths); err != nil {
		t.Fatalf("invalid paths response: %v", err)
	}
	if paths.DataDir != root {
		t.Fatalf("data dir = %q, want %q", paths.DataDir, root)
	}
	for _, dir := range []string{paths.CacheDir, paths.ProjectDir, paths.ArtifactDir, paths.LogDir, paths.DiagnosticsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %q to exist, stat=%v err=%v", dir, info, err)
		}
	}
}

func TestLocalArtifactStoreSupportsBinaryPayloads(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	payload := []byte{0x00, 0x01, 0x02, 0xff}
	body := bytes.NewBufferString(`{
		"id":"img-1",
		"projectId":"vp-1",
		"storageRef":"local://projects/vp-1/artifacts/keyframe/image/hash/keyframe.png",
		"mimeType":"image/png",
		"contentBase64":"` + base64.StdEncoding.EncodeToString(payload) + `",
		"metadata":{"kind":"IMAGE","cloudPayloadStored":false}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stored LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatalf("invalid store response: %v", err)
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("expected local binary artifact: %v", err)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("binary payload mismatch: %#v", content)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/artifacts/img-1?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var loaded LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("invalid read response: %v", err)
	}
	if loaded.Content != "" {
		t.Fatalf("binary response should not use text content: %q", loaded.Content)
	}
	if loaded.ContentBase64 != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("binary response base64 mismatch: %q", loaded.ContentBase64)
	}
}

func TestModelProviderSettingsSaveListAndPreserveSecrets(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"baseUrl":"https://api.openai.com/v1","model":"gpt-4.1","apiKey":"sk-text-secret"},
			"text_to_image":{"baseUrl":"https://api.example.com/v1","model":"gpt-image-1","apiKey":"sk-image-secret"},
			"text_to_video":{"baseUrl":"https://video.example.com/v1","model":"seedance-v1","apiKey":"sk-video-secret"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	configFile := filepath.Join(root, "config", "model-providers.json")
	raw, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("expected provider config file: %v", err)
	}
	if !bytes.Contains(raw, []byte("sk-text-secret")) {
		t.Fatalf("local config should store the full local token")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/model-providers", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-text-secret")) || bytes.Contains(rec.Body.Bytes(), []byte("sk-image-secret")) {
		t.Fatalf("provider settings GET should not expose full api keys: %s", rec.Body.String())
	}
	var listed ModelProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("invalid provider response: %v", err)
	}
	if !listed.Providers[CapabilityTextToText].HasAPIKey {
		t.Fatalf("text provider should report existing api key: %+v", listed.Providers[CapabilityTextToText])
	}
	if listed.Providers[CapabilityTextToText].APIKeyPreview == "" {
		t.Fatalf("text provider should include masked key preview")
	}

	body = bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"baseUrl":"https://proxy.example.com/v1","model":"gpt-4.1-mini","apiKey":""}
		}
	}`)
	req = httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	raw, err = os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	if !bytes.Contains(raw, []byte("sk-text-secret")) {
		t.Fatalf("blank apiKey should preserve existing local token: %s", string(raw))
	}
	if !bytes.Contains(raw, []byte("https://proxy.example.com/v1")) {
		t.Fatalf("baseUrl should be updated: %s", string(raw))
	}
}

func TestModelProviderSettingsRejectUnsupportedCapability(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})
	body := bytes.NewBufferString(`{
		"providers":{
			"image_to_video":{"baseUrl":"https://example.com/v1","model":"bad","apiKey":"sk"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported capability should return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBiaoshuProjectStoreUpsertListAndRejectUnsafeRunID(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	body := bytes.NewBufferString(`{
		"projectName":"广惠高速改扩建",
		"bidFilePath":"E:\\bid\\test_bid.pdf",
		"status":"RUNNING",
		"createdAt":"2026-06-30T01:00:00Z",
		"updatedAt":"2026-06-30T01:05:00Z",
		"artifactCount":6,
		"validArtifactCount":2
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/biaoshu-projects/run-1", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert status = %d, body = %s", rec.Code, rec.Body.String())
	}

	indexPath := filepath.Join(root, "projects", "biaoshu-projects.json")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected biaoshu project index: %v", err)
	}
	if !bytes.Contains(raw, []byte("广惠高速改扩建")) {
		t.Fatalf("project index missing project name: %s", string(raw))
	}

	body = bytes.NewBufferString(`{
		"projectName":"广惠高速改扩建",
		"bidFilePath":"E:\\bid\\test_bid.pdf",
		"status":"SUCCESS",
		"updatedAt":"2026-06-30T01:10:00Z",
		"artifactCount":6,
		"validArtifactCount":6
	}`)
	req = httptest.NewRequest(http.MethodPut, "/api/local/biaoshu-projects/run-1", body)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/biaoshu-projects", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listed BiaoshuProjectListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("invalid biaoshu project list: %v", err)
	}
	if len(listed.Projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(listed.Projects))
	}
	got := listed.Projects[0]
	if got.Status != "SUCCESS" || got.ValidArtifactCount != 6 {
		t.Fatalf("project not updated: %+v", got)
	}
	if got.CreatedAt != "2026-06-30T01:00:00Z" {
		t.Fatalf("createdAt should be preserved, got %q", got.CreatedAt)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/local/biaoshu-projects/bad%5Crun", bytes.NewBufferString(`{}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsafe run id should return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReadBiaoshuArtifactReadsTrustedTextFileAndRejectsOutsidePath(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	artifactPath := filepath.Join(root, "biaoshu-output", "analysis.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("# 招标文件解析\n\n正文"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	body := bytes.NewBufferString(fmt.Sprintf(`{"filePath":%q}`, artifactPath))
	req := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu-artifacts/read", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		FilePath string `json:"filePath"`
		Format   string `json:"format"`
		Content  string `json:"content"`
		Size     int64  `json:"size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid read response: %v", err)
	}
	if resp.FilePath != artifactPath || resp.Format != "md" || resp.Content != "# 招标文件解析\n\n正文" {
		t.Fatalf("unexpected read response: %+v", resp)
	}

	outsidePath := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	body = bytes.NewBufferString(fmt.Sprintf(`{"filePath":%q}`, outsidePath))
	req = httptest.NewRequest(http.MethodPost, "/api/local/biaoshu-artifacts/read", body)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("outside path should return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteLogAndCreateDiagnostics(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	body := bytes.NewBufferString(`{"source":"desktop","level":"info","message":"render complete","fields":{"project":"demo"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/local/logs", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("log status = %d, body = %s", rec.Code, rec.Body.String())
	}
	logFile := filepath.Join(root, "logs", "local-agent.jsonl")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected log file: %v", err)
	}
	if !bytes.Contains(content, []byte("render complete")) {
		t.Fatalf("log file missing message: %s", string(content))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/diagnostics", bytes.NewBufferString(`{"reason":"support-request"}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp DiagnosticResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid diagnostics response: %v", err)
	}
	if resp.Path == "" {
		t.Fatalf("diagnostics path is empty")
	}
	zr, err := zip.OpenReader(resp.Path)
	if err != nil {
		t.Fatalf("diagnostics zip not readable: %v", err)
	}
	defer zr.Close()
	var hasManifest, hasLog bool
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			hasManifest = true
		}
		if f.Name == "logs/local-agent.jsonl" {
			hasLog = true
		}
	}
	if !hasManifest || !hasLog {
		t.Fatalf("diagnostics zip missing files: manifest=%v log=%v", hasManifest, hasLog)
	}
}

func TestHandlerAllowsLocalFrontendCORS(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})

	req := httptest.NewRequest(http.MethodOptions, "/api/local/diagnostics", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow-origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPut) {
		t.Fatalf("allow-methods = %q, want PUT", got)
	}
}

func TestLocalArtifactStoreWritesAndReadsUserPayload(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"id":"art-1",
		"projectId":"vp-1",
		"storageRef":"local://projects/vp-1/artifacts/script/content/hash/script.md",
		"mimeType":"text/markdown; charset=utf-8",
		"content":"## 本地脚本\n用户资产只保存在本地。",
		"metadata":{"stageName":"script","cloudPayloadStored":false}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stored LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatalf("invalid store response: %v", err)
	}
	if stored.Path == "" || stored.MetadataPath == "" {
		t.Fatalf("expected local paths in response: %+v", stored)
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("expected local artifact payload: %v", err)
	}
	if !bytes.Contains(content, []byte("用户资产只保存在本地")) {
		t.Fatalf("artifact payload mismatch: %s", string(content))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/artifacts/art-1?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var loaded LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("invalid read response: %v", err)
	}
	if loaded.Content != "## 本地脚本\n用户资产只保存在本地。" {
		t.Fatalf("loaded content mismatch: %q", loaded.Content)
	}
	if loaded.Metadata["cloudPayloadStored"] != false {
		t.Fatalf("metadata should preserve cloudPayloadStored=false: %+v", loaded.Metadata)
	}
}

func TestLocalArtifactDeleteRemovesPayloadAndMetadata(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"id":"art-delete",
		"projectId":"vp-1",
		"storageRef":"local://projects/vp-1/artifacts/script/content/hash/script.md",
		"mimeType":"text/markdown",
		"content":"delete me"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/local/artifacts/art-delete?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(root, "artifacts", "vp-1", "art-delete")); !os.IsNotExist(err) {
		t.Fatalf("artifact directory should be deleted, err=%v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/artifacts/art-delete?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted artifact should return 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalProjectDeleteRemovesProjectArtifactsAndCache(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	for _, dir := range []string{
		filepath.Join(root, "projects", "vp-1"),
		filepath.Join(root, "artifacts", "vp-1", "art-1"),
		filepath.Join(root, "cache", "vp-1"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data"), []byte("payload"), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/local/projects/vp-1", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("project delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, dir := range []string{
		filepath.Join(root, "projects", "vp-1"),
		filepath.Join(root, "artifacts", "vp-1"),
		filepath.Join(root, "cache", "vp-1"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("project delete should remove %s, err=%v", dir, err)
		}
	}
}
