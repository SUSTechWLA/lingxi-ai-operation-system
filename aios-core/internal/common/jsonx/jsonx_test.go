package jsonx

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractJSON_CleanObject(t *testing.T) {
	raw := `{"title": "hello", "count": 42}`
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "hello" {
		t.Errorf("expected title='hello', got %v", result["title"])
	}
	if result["count"].(float64) != 42 {
		t.Errorf("expected count=42, got %v", result["count"])
	}
}

func TestExtractJSON_CleanArray(t *testing.T) {
	raw := `["keyword1", "keyword2"]`
	var result []string
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 || result[0] != "keyword1" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestExtractJSON_MarkdownFenceWithLang(t *testing.T) {
	raw := "```json\n{\"title\": \"hello\"}\n```"
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "hello" {
		t.Errorf("expected title='hello', got %v", result["title"])
	}
}

func TestExtractJSON_MarkdownFenceNoLang(t *testing.T) {
	raw := "```\n{\"title\": \"hello\"}\n```"
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "hello" {
		t.Errorf("expected title='hello', got %v", result["title"])
	}
}

func TestExtractJSON_TextBeforeJSON(t *testing.T) {
	raw := "Here is the result:\n{\"title\": \"hello\"}"
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "hello" {
		t.Errorf("expected title='hello', got %v", result["title"])
	}
}

func TestExtractJSON_TextAfterJSON(t *testing.T) {
	raw := "{\"title\": \"hello\"}\nHope this helps!"
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "hello" {
		t.Errorf("expected title='hello', got %v", result["title"])
	}
}

func TestExtractJSON_TextBeforeArray(t *testing.T) {
	raw := "Here are the keywords: [\"k1\", \"k2\"]"
	var result []string
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 || result[0] != "k1" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestExtractJSON_TrailingCommaInObject(t *testing.T) {
	raw := `{"title": "hello", "count": 42,}`
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "hello" {
		t.Errorf("expected title='hello', got %v", result["title"])
	}
}

func TestExtractJSON_TrailingCommaInArray(t *testing.T) {
	raw := `["a", "b",]`
	var result []string
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
}

func TestExtractJSON_NestedObjectsWithText(t *testing.T) {
	raw := "Result: {\"outer\": {\"inner\": [1, 2, 3]}} Done."
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outer, ok := result["outer"].(map[string]interface{})
	if !ok {
		t.Fatal("expected outer to be a map")
	}
	inner, ok := outer["inner"].([]interface{})
	if !ok || len(inner) != 3 {
		t.Errorf("expected inner=[1,2,3], got %v", outer["inner"])
	}
}

func TestExtractJSON_MarkdownFenceInMiddle(t *testing.T) {
	raw := "Some text\n```json\n{\"key\": \"value\"}\n```\nMore text"
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key='value', got %v", result["key"])
	}
}

func TestExtractJSON_EmptyString(t *testing.T) {
	var result map[string]interface{}
	err := ExtractJSON("", &result)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	var result map[string]interface{}
	err := ExtractJSON("just some plain text without any json", &result)
	if err == nil {
		t.Fatal("expected error for non-JSON input")
	}
}

func TestExtractJSON_StringWithEscapedQuotes(t *testing.T) {
	raw := `{"text": "he said \"hello\""}`
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["text"] != `he said "hello"` {
		t.Errorf("unexpected text: %v", result["text"])
	}
}

func TestExtractJSON_TargetStruct(t *testing.T) {
	raw := "```json\n{\"title\": \"Test Title\", \"description\": \"A description\", \"keywords\": [\"kw1\", \"kw2\"]}\n```"
	type ChatFields struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
	}
	var fields ChatFields
	if err := ExtractJSON(raw, &fields); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields.Title != "Test Title" {
		t.Errorf("expected Title='Test Title', got %q", fields.Title)
	}
	if len(fields.Keywords) != 2 {
		t.Errorf("expected 2 keywords, got %d", len(fields.Keywords))
	}
}

func TestExtractJSON_RealWorldKeywordsArray(t *testing.T) {
	// Simulates the actual issue: keywords returned as bare JSON array
	raw := `["躺营AI", "躺营AI自媒体运营助手", "AI自媒体运营工具"]`
	var keywords []string
	if err := ExtractJSON(raw, &keywords); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keywords) != 3 {
		t.Errorf("expected 3 keywords, got %d", len(keywords))
	}
	if keywords[0] != "躺营AI" {
		t.Errorf("expected first keyword='躺营AI', got %q", keywords[0])
	}
}

func TestExtractJSON_UnescapedNewlinesInStrings(t *testing.T) {
	// Simulates the exact ChatReviseTool scenario: LLM returns JSON with
	// literal newlines inside the description string value.
	raw := `{
  "type": "revise",
  "reply": "已将原有营销感较强的简介修改",
  "fields": {
    "title": "",
    "description": "之前为了提高工作效率少加班、规划转AI赛道我找了好多相关课程。
从零基础的AI认知、大模型底层原理讲起，内容覆盖提示词工程。
我自己学了半个多月，现在日常处理报表、写方案起码省了一半时间。",
    "keywords": []
  }
}`

	type reviseOutput struct {
		Type   string `json:"type"`
		Reply  string `json:"reply"`
		Fields struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Keywords    []string `json:"keywords"`
		} `json:"fields"`
	}

	var parsed reviseOutput
	if err := ExtractJSON(raw, &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Type != "revise" {
		t.Errorf("expected type='revise', got %q", parsed.Type)
	}
	if parsed.Fields.Title != "" {
		t.Errorf("expected empty title, got %q", parsed.Fields.Title)
	}
	if parsed.Fields.Description == "" {
		t.Fatal("expected non-empty description, got empty")
	}
	if !strings.Contains(parsed.Fields.Description, "少加班") {
		t.Errorf("description missing expected content, got: %s", parsed.Fields.Description)
	}
	// Verify newlines became literal \n in the Go string value
	if !strings.Contains(parsed.Fields.Description, "\n") {
		t.Error("expected newlines preserved in description")
	}
}

func TestExtractJSON_UnescapedTabs(t *testing.T) {
	raw := "{\"text\": \"hello\tworld\"}"
	var result map[string]interface{}
	if err := ExtractJSON(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["text"] != "hello\tworld" {
		t.Errorf("expected tab preserved, got %q", result["text"])
	}
}

// roundTripJSON is a helper to produce compact JSON strings in tests
func roundTripJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return string(b)
}
