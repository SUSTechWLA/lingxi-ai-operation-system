package localagent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
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
	if stored.Metadata["sourceType"] != "uploaded" || stored.Metadata["providerName"] != "local-upload" || stored.Metadata["isFallback"] != false {
		t.Fatalf("uploaded artifact should get default provenance fields: %+v", stored.Metadata)
	}
	provenance, ok := stored.Metadata["provenance"].(map[string]interface{})
	if !ok {
		t.Fatalf("uploaded artifact should include nested provenance: %+v", stored.Metadata)
	}
	for _, key := range []string{"schemaVersion", "sourceType", "providerName", "providerJobId", "fallbackReason", "isFallback", "generatedAt", "inputPromptHash", "sourceArtifactIds"} {
		if _, exists := provenance[key]; !exists {
			t.Fatalf("uploaded provenance missing %s: %+v", key, provenance)
		}
	}
	if provenance["sourceType"] != "uploaded" || provenance["providerName"] != "local-upload" {
		t.Fatalf("unexpected uploaded provenance: %+v", provenance)
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

type fakeAgentCommandRunner struct {
	calls []fakeAgentCommandCall
	out   CommandOutput
	err   error
}

type fakeAgentCommandCall struct {
	name string
	args []string
}

func (r *fakeAgentCommandRunner) Run(_ context.Context, name string, args ...string) (CommandOutput, error) {
	r.calls = append(r.calls, fakeAgentCommandCall{name: name, args: append([]string(nil), args...)})
	return r.out, r.err
}

func TestReadMCPProvidersBootstrapsBundledIPAvatarWhenConfigIsMissing(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "repo", "mcp", "ip_avatar_3d", "server.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatalf("mkdir MCP script dir: %v", err)
	}
	if err := os.WriteFile(script, []byte("# test MCP server\n"), 0o644); err != nil {
		t.Fatalf("write MCP script: %v", err)
	}
	t.Setenv("TANGYING_IP_AVATAR_MCP_SCRIPT", script)

	server := NewServer(Config{DataDir: filepath.Join(root, "data")})
	providers, err := server.ReadMCPProviders()
	if err != nil {
		t.Fatalf("ReadMCPProviders: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("providers = %#v, want bundled IP avatar provider", providers)
	}
	provider := providers[0]
	if provider.ID != "ip_avatar_3d" || provider.Transport != "stdio" || provider.ToolPrefix != "ip_avatar_3d." || !provider.Enabled {
		t.Fatalf("unexpected IP avatar provider: %+v", provider)
	}
	if len(provider.Args) != 1 || provider.Args[0] != script || provider.TimeoutSec != 3600 {
		t.Fatalf("unexpected IP avatar command contract: %+v", provider)
	}
}

func TestLocalMCPProviderSettingsSaveAndStatus(t *testing.T) {
	root := t.TempDir()
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode mcp request: %v", err)
		}
		if req["method"] != "tools/list" {
			t.Fatalf("method = %v, want tools/list", req["method"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "jimeng.generate_video", "description": "generate video"},
				},
			},
		})
	}))
	defer mcp.Close()

	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{"providers":[{"id":"jimeng","label":"JiMeng","endpoint":"` + mcp.URL + `","enabled":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers/status", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var status LocalMCPProviderStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(status.Providers) != 1 {
		t.Fatalf("provider count = %d, want 1", len(status.Providers))
	}
	if !status.Providers[0].Reachable {
		t.Fatalf("provider should be reachable: %+v", status.Providers[0])
	}
	if len(status.Providers[0].Tools) != 1 || status.Providers[0].Tools[0].Name != "jimeng.generate_video" {
		t.Fatalf("tools = %+v", status.Providers[0].Tools)
	}
}

func TestLocalMCPProviderSettingsAcceptsStandardStdioProvider(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":[{
			"id":"echo",
			"label":"Echo MCP",
				"transport":"stdio",
				"command":"python3",
				"args":["/opt/mcp/echo_server.py"],
				"env":{"ECHO_MODE":"test"},
				"toolPrefix":"echo.",
				"toolNameMap":{"echo.health":"health"},
				"enabled":true
			}]
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save stdio provider status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var listed LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("invalid mcp provider response: %v", err)
	}
	if len(listed.Providers) != 1 {
		t.Fatalf("providers = %#v, want one", listed.Providers)
	}
	provider := listed.Providers[0]
	if provider.Transport != "stdio" || provider.Command != "python3" || provider.Endpoint != "" {
		t.Fatalf("stdio provider not normalized as expected: %+v", provider)
	}
	if len(provider.Args) != 1 || provider.Args[0] != "/opt/mcp/echo_server.py" {
		t.Fatalf("stdio args not preserved: %+v", provider.Args)
	}
	if provider.Env["ECHO_MODE"] != "test" {
		t.Fatalf("stdio env not preserved: %+v", provider.Env)
	}
	if provider.ToolPrefix != "echo." || provider.ToolNameMap["echo.health"] != "health" {
		t.Fatalf("stdio tool mapping not preserved: %+v", provider)
	}
}

func TestJiMengSetupStatusReadsDreaminaStatusThroughMCP(t *testing.T) {
	root := t.TempDir()
	runner := &fakeAgentCommandRunner{}
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode mcp request: %v", err)
		}
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": req["id"]}
		switch req["method"] {
		case "tools/list":
			resp["result"] = map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "jimeng.check_status", "description": "check status"},
					{"name": "jimeng.generate_video", "description": "generate video"},
				},
			}
		case "tools/call":
			params := req["params"].(map[string]interface{})
			if params["name"] != "jimeng.check_status" {
				t.Fatalf("tool = %v, want jimeng.check_status", params["name"])
			}
			resp["result"] = map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"available": true,
					"version":   "dreamina-from-mcp",
				},
			}
		default:
			t.Fatalf("unexpected mcp method %v", req["method"])
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mcp.Close()

	server := NewServer(Config{DataDir: root, CommandRunner: runner})
	body := bytes.NewBufferString(`{"providers":[{"id":"jimeng","label":"JiMeng","endpoint":"` + mcp.URL + `","enabled":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/jimeng/setup/status", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var status JiMengSetupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	if !status.DreaminaAvailable || status.DreaminaVersion != "dreamina-from-mcp" {
		t.Fatalf("dreamina status = available:%v version:%q, want MCP result", status.DreaminaAvailable, status.DreaminaVersion)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("setup status must not call Dreamina CLI directly: %+v", runner.calls)
	}
}

func TestJiMengInstallCLIRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	runner := &fakeAgentCommandRunner{out: CommandOutput{Stdout: "installed"}}
	server := NewServer(Config{DataDir: root, CommandRunner: runner})

	req := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/install-cli", bytes.NewBufferString(`{"confirm":false}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("install should not run without confirmation: %+v", runner.calls)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/install-cli", bytes.NewBufferString(`{"confirm":true}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("install calls = %d, want 1", len(runner.calls))
	}
	if runner.calls[0].name != "sh" || runner.calls[0].args[0] != "-c" {
		t.Fatalf("install command = %s %#v", runner.calls[0].name, runner.calls[0].args)
	}
	if !strings.Contains(runner.calls[0].args[1], "https://jimeng.jianying.com/cli") {
		t.Fatalf("install script URL missing: %#v", runner.calls[0].args)
	}
}

func TestJiMengRegisterMCPStoresDefaultProvider(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	req := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/register-mcp", bytes.NewBufferString(`{"endpoint":"http://127.0.0.1:18180"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get providers status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listed LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	provider, ok := localMCPProviderByID(listed.Providers, "jimeng")
	if !ok || !provider.Enabled {
		t.Fatalf("providers = %+v", listed.Providers)
	}
}

func TestJiMengRegisterMCPStoresDefaultStandardStdioProvider(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	req := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/register-mcp", bytes.NewBufferString(`{"transport":"stdio"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register stdio status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get providers status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listed LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	provider, ok := localMCPProviderByID(listed.Providers, "jimeng")
	if !ok {
		t.Fatalf("providers = %+v", listed.Providers)
	}
	if provider.ID != "jimeng" || provider.Transport != "stdio" || provider.Command == "" || len(provider.Args) == 0 {
		t.Fatalf("stdio provider not registered as expected: %+v", provider)
	}
	if provider.ToolPrefix != "jimeng." {
		t.Fatalf("tool prefix = %q, want jimeng.", provider.ToolPrefix)
	}
	if provider.Endpoint != "" {
		t.Fatalf("stdio provider endpoint = %q, want empty", provider.Endpoint)
	}
	if !strings.HasSuffix(provider.Args[0], filepath.Join("mcp", "jimeng", "server.py")) {
		t.Fatalf("stdio script arg = %q, want jimeng server.py", provider.Args[0])
	}
}

func localMCPProviderByID(providers []localmcp.ProviderConfig, id string) (localmcp.ProviderConfig, bool) {
	for _, provider := range providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return localmcp.ProviderConfig{}, false
}

func TestJiMengLoginHeadlessCallsRegisteredMCPProvider(t *testing.T) {
	root := t.TempDir()
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode mcp request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		if params["name"] != "jimeng.login_headless" {
			t.Fatalf("tool = %v, want jimeng.login_headless", params["name"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"verification_uri": "https://example.com/device",
					"user_code":        "ABCD-EFGH",
					"device_code":      "device-1",
				},
			},
		})
	}))
	defer mcp.Close()

	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{"providers":[{"id":"jimeng","label":"JiMeng","endpoint":"` + mcp.URL + `","enabled":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/login-headless", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	structured := result["structuredContent"].(map[string]interface{})
	if structured["user_code"] != "ABCD-EFGH" {
		t.Fatalf("user_code = %#v", structured["user_code"])
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
	t.Setenv("TANGYING_APP_VERSION", "9.9.9-beta")
	t.Setenv("TANGYING_GIT_COMMIT", "abc1234-test")

	body := bytes.NewBufferString(`{"source":"desktop","level":"info","message":"render complete","fields":{"project":"demo","taskId":"task-beta-123","authorization":"Bearer live-token-should-redact","cookie":"session-cookie-should-redact"}}`)
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
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "mcp-providers.json"), []byte(`{"providers":[{"id":"jimeng","label":"JiMeng MCP","transport":"stdio","command":"python3","args":["mcp/jimeng/server.py"],"env":{"DREAMINA_TOKEN":"secret-token"},"enabled":true}]}`), 0o644); err != nil {
		t.Fatalf("write mcp providers: %v", err)
	}
	artifactDir := filepath.Join(root, "artifacts", "vp-1", "video-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("raw-video-bytes-should-not-be-zipped"), 0o644); err != nil {
		t.Fatalf("write artifact content: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), []byte(`{"id":"video-1","projectId":"vp-1","kind":"VIDEO","sourceType":"fallback_preview","isFallback":true,"fallbackReason":"no_ready_aigc_video","storageRef":"local://projects/vp-1/artifacts/video-1/hash/final.mp4"}`), 0o644); err != nil {
		t.Fatalf("write artifact metadata: %v", err)
	}
	reportDir := filepath.Join(root, "projects", "vp-1", "reports", "video_frame_qa")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("create report dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "shot_qa_reports.json"), []byte(`{"schemaVersion":1,"shotReports":[{"shotId":"SHOT_01","decision":"HUMAN_REVIEW"}]}`), 0o644); err != nil {
		t.Fatalf("write QA report: %v", err)
	}
	pipelineReports := map[string]string{
		"shot_list.json":                `{"shots":[{"shotId":"SHOT_01"}]}`,
		"shot_split_report.json":        `{"policy":{"minShotDurationSec":3,"maxShotDurationSec":15}}`,
		"shot_duration_validation.json": `{"valid":true}`,
		"shot_candidates.json":          `{"candidates":[{"candidateId":"cand-1"}]}`,
		"repair_plans.json":             `{"repairPlans":[{"action":"RERENDER_HTML"}]}`,
		"accepted_shots.json":           `{"acceptedShots":[{"shotId":"SHOT_01","candidateId":"cand-2"}]}`,
		"assembly_plan.json":            `{"steps":["FFMPEG_CONCAT","GLOBAL_SUBTITLE_RENDER"]}`,
		"subtitle_timeline.json":        `{"scope":"global","cues":[]}`,
		"audio_mix_plan.json":           `{"scope":"global","bgmDucking":true}`,
		"final_qa_report.json":          `{"passed":true}`,
		"provenance_summary.json":       `{"fallbackCount":1}`,
	}
	for name, body := range pipelineReports {
		if err := os.WriteFile(filepath.Join(reportDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write pipeline report %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "failure-stack.txt"), []byte("panic: render failed\nsk-test-secret-should-redact\n"), 0o644); err != nil {
		t.Fatalf("write failure stack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "provider-failure.json"), []byte(`{"authorization":"Bearer json-bearer-should-redact","cookie":"json-cookie-should-redact","message":"provider failed"}`), 0o644); err != nil {
		t.Fatalf("write json failure stack: %v", err)
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
	if filepath.Base(resp.Path) != "beta-diagnostics.zip" {
		t.Fatalf("diagnostics filename = %q, want beta-diagnostics.zip", filepath.Base(resp.Path))
	}
	zr, err := zip.OpenReader(resp.Path)
	if err != nil {
		t.Fatalf("diagnostics zip not readable: %v", err)
	}
	defer zr.Close()
	entries := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, err)
		}
		entries[f.Name] = string(data)
	}
	for _, want := range []string{
		"manifest.json",
		"environment.json",
		"env-redacted.json",
		"mcp/provider-status.json",
		"artifacts/manifest.json",
		"qa/shot_qa_reports.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_list.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_split_report.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_duration_validation.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_candidates.json",
		"pipeline/vp-1/reports/video_frame_qa/repair_plans.json",
		"pipeline/vp-1/reports/video_frame_qa/accepted_shots.json",
		"pipeline/vp-1/reports/video_frame_qa/assembly_plan.json",
		"pipeline/vp-1/reports/video_frame_qa/subtitle_timeline.json",
		"pipeline/vp-1/reports/video_frame_qa/audio_mix_plan.json",
		"pipeline/vp-1/reports/video_frame_qa/final_qa_report.json",
		"pipeline/vp-1/reports/video_frame_qa/provenance_summary.json",
		"failures/failure-stack.txt",
		"logs/local-agent.jsonl",
	} {
		if _, ok := entries[want]; !ok {
			t.Fatalf("diagnostics zip missing %s; entries=%v", want, entries)
		}
	}
	if _, ok := entries["artifacts/vp-1/video-1/content"]; ok {
		t.Fatalf("diagnostics zip must not include raw artifact content")
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal([]byte(entries["manifest.json"]), &manifest); err != nil {
		t.Fatalf("invalid diagnostics manifest: %v", err)
	}
	if manifest["appVersion"] != "9.9.9-beta" || manifest["gitCommit"] != "abc1234-test" {
		t.Fatalf("diagnostics manifest should include version and git commit: %#v", manifest)
	}
	recentTaskIDs, ok := manifest["recentTaskIds"].([]interface{})
	if !ok || len(recentTaskIDs) != 1 || recentTaskIDs[0] != "task-beta-123" {
		t.Fatalf("diagnostics manifest should include recent task ids, got %#v", manifest["recentTaskIds"])
	}
	allEntries := strings.Join(mapValues(entries), "\n")
	if strings.Contains(allEntries, "secret-token") ||
		strings.Contains(allEntries, "sk-test-secret-should-redact") ||
		strings.Contains(allEntries, "live-token-should-redact") ||
		strings.Contains(allEntries, "session-cookie-should-redact") ||
		strings.Contains(allEntries, "json-bearer-should-redact") ||
		strings.Contains(allEntries, "json-cookie-should-redact") {
		t.Fatalf("diagnostics zip leaked a secret: %#v", entries)
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
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Range") {
		t.Fatalf("allow-headers = %q, want Range", got)
	}
}

func TestHandlerRejectsUntrustedOriginForArtifactWrite(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})
	body := bytes.NewBufferString(`{
		"id":"art-evil",
		"projectId":"vp-evil",
		"content":"blocked"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("untrusted origin status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerAllowsLocalOriginForArtifactWrite(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})
	body := bytes.NewBufferString(`{
		"id":"art-local",
		"projectId":"vp-local",
		"content":"allowed"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("local origin status = %d, body = %s", rec.Code, rec.Body.String())
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

	req = httptest.NewRequest(http.MethodHead, "/api/local/artifacts/video-1?projectId=vp-1&raw=1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("raw head status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "video/mp4") {
		t.Fatalf("raw head content-type = %q", contentType)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("raw head should not write body, got %d bytes", rec.Body.Len())
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

func mapValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
