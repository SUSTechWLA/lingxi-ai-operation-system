package localagent

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
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

func TestLocalArtifactStoreAcceptsMultipartUpload(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	payload := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("id", "extgen-video-1"); err != nil {
		t.Fatalf("write id field: %v", err)
	}
	if err := writer.WriteField("projectId", "vp-1"); err != nil {
		t.Fatalf("write project field: %v", err)
	}
	if err := writer.WriteField("mimeType", "video/mp4"); err != nil {
		t.Fatalf("write mime field: %v", err)
	}
	if err := writer.WriteField("metadata", `{"artifactType":"external_generation_result","externalGenerationRequestId":"extgen_123","cloudPayloadStored":false}`); err != nil {
		t.Fatalf("write metadata field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "shot-1.mp4")
	if err != nil {
		t.Fatalf("create upload part: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write upload payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stored LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatalf("invalid upload response: %v", err)
	}
	if !strings.HasPrefix(stored.StorageRef, "local://projects/vp-1/artifacts/extgen-video-1/") {
		t.Fatalf("unexpected storageRef: %q", stored.StorageRef)
	}
	if !strings.HasPrefix(stored.ContentHash, "sha256:") {
		t.Fatalf("expected sha256 content hash, got %q", stored.ContentHash)
	}
	if stored.SizeBytes != int64(len(payload)) {
		t.Fatalf("sizeBytes = %d, want %d", stored.SizeBytes, len(payload))
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("expected uploaded artifact payload: %v", err)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("uploaded payload mismatch: %#v", content)
	}
	if stored.Metadata["externalGenerationRequestId"] != "extgen_123" {
		t.Fatalf("metadata should preserve request link: %+v", stored.Metadata)
	}
	if stored.Metadata["contentHash"] != stored.ContentHash {
		t.Fatalf("metadata should include returned hash: %+v", stored.Metadata)
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

func TestModelProviderSettingsIncludeKeyRequiresNonBrowserLocalRequest(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"baseUrl":"https://api.openai.com/v1","model":"gpt-4.1","apiKey":"sk-text-secret"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/model-providers?include_key=true", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("local include_key status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("sk-text-secret")) {
		t.Fatalf("non-browser localhost request should include full key: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/model-providers?include_key=true", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("Origin", "http://localhost:3000")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("browser include_key status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-text-secret")) {
		t.Fatalf("browser-origin request must not expose full api key: %s", rec.Body.String())
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
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q, want local frontend origin", got)
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

func TestLocalArtifactRawReturnsMediaBytes(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	artifactDir := filepath.Join(root, "artifacts", "vp-1", "video-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	content := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	if err := os.WriteFile(filepath.Join(artifactDir, "content"), content, 0o644); err != nil {
		t.Fatalf("write content: %v", err)
	}
	metadata := []byte(`{"id":"video-1","projectId":"vp-1","mimeType":"video/mp4","storageRef":"local://projects/vp-1/artifacts/video-1/hash/final.mp4"}`)
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), metadata, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local/artifacts/video-1?projectId=vp-1&raw=1", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("raw status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "video/mp4") {
		t.Fatalf("raw content-type = %q", contentType)
	}
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatalf("raw content mismatch: %v", rec.Body.Bytes())
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
