package jimengmcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerListsJiMengTools(t *testing.T) {
	server := httptest.NewServer(NewServer(NewAdapter(AdapterConfig{Runner: &fakeRunner{}})))
	defer server.Close()

	body := postRPC(t, server.URL, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "tools/list",
	})
	result := body["result"].(map[string]interface{})
	tools := result["tools"].([]interface{})
	if len(tools) == 0 {
		t.Fatal("tools list is empty")
	}
	if tools[0].(map[string]interface{})["name"] != "jimeng.check_status" {
		t.Fatalf("first tool = %#v, want jimeng.check_status", tools[0])
	}
}

func TestServerCallsGenerateVideoTool(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: `{"submit_id":"vid-1","gen_status":"querying"}`}}
	server := httptest.NewServer(NewServer(NewAdapter(AdapterConfig{Runner: runner})))
	defer server.Close()

	body := postRPC(t, server.URL, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "jimeng.generate_video",
			"arguments": map[string]interface{}{
				"mode":             "text2video",
				"prompt":           "wide shot",
				"duration":         5,
				"ratio":            "16:9",
				"video_resolution": "720p",
				"poll":             30,
			},
		},
	})
	result := body["result"].(map[string]interface{})
	if result["isError"] == true {
		t.Fatalf("result is error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]interface{})
	if structured["submit_id"] != "vid-1" {
		t.Fatalf("submit_id = %v, want vid-1", structured["submit_id"])
	}
	want := []string{"text2video", "--prompt=wide shot", "--duration=5", "--ratio=16:9", "--video_resolution=720p", "--poll=30"}
	if len(runner.calls) != 1 || stringsJoin(runner.calls[0].args) != stringsJoin(want) {
		t.Fatalf("args = %#v, want %#v", runner.calls, want)
	}
}

func TestServerReturnsToolErrorsAsMCPResult(t *testing.T) {
	runner := &fakeRunner{out: CommandOutput{Stdout: `{"submit_id":"bad","gen_status":"fail","fail_reason":"blocked"}`}}
	server := httptest.NewServer(NewServer(NewAdapter(AdapterConfig{Runner: runner})))
	defer server.Close()

	body := postRPC(t, server.URL, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "3",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "jimeng.generate_image",
			"arguments": map[string]interface{}{"prompt": "x"},
		},
	})
	result := body["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Fatalf("isError = %v, want true", result["isError"])
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if text != "dreamina generation failed: blocked" {
		t.Fatalf("error text = %q", text)
	}
}

func postRPC(t *testing.T, url string, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post rpc: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode rpc response: %v", err)
	}
	if out["error"] != nil {
		t.Fatalf("rpc error: %#v", out["error"])
	}
	return out
}

func stringsJoin(values []string) string {
	var buf bytes.Buffer
	for _, value := range values {
		buf.WriteString(value)
		buf.WriteByte('\n')
	}
	return buf.String()
}
