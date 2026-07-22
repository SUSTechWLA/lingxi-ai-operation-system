package localmcp

import "encoding/json"

const (
	ApprovalModeNone          = "none"
	ApprovalModeBeforeExecute = "before_execute"
	ApprovalModeAlways        = "always"
)

type ProviderConfig struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Endpoint      string            `json:"endpoint,omitempty"`
	Transport     string            `json:"transport,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	HasHeaders    bool              `json:"hasHeaders,omitempty"`
	HeaderKeys    []string          `json:"headerKeys,omitempty"`
	WorkingDir    string            `json:"workingDir,omitempty"`
	ToolPrefix    string            `json:"toolPrefix,omitempty"`
	ToolNameMap   map[string]string `json:"toolNameMap,omitempty"`
	Enabled       bool              `json:"enabled"`
	EnabledTools  []string          `json:"enabledTools,omitempty"`
	DisabledTools []string          `json:"disabledTools,omitempty"`
	TimeoutSec    int               `json:"timeout,omitempty"`
	ApprovalMode  string            `json:"approvalMode,omitempty"`
}

// Tool is the lossless local representation of an MCP tool definition. Raw
// retains extension and future protocol fields while the named fields keep
// existing callers ergonomic.
type Tool struct {
	Name         string                   `json:"name"`
	Title        string                   `json:"title,omitempty"`
	Description  string                   `json:"description,omitempty"`
	InputSchema  map[string]interface{}   `json:"inputSchema"`
	OutputSchema map[string]interface{}   `json:"outputSchema,omitempty"`
	Annotations  map[string]interface{}   `json:"annotations,omitempty"`
	Icons        []map[string]interface{} `json:"icons,omitempty"`
	Meta         map[string]interface{}   `json:"_meta,omitempty"`
	Raw          map[string]interface{}   `json:"-"`
}

func (t *Tool) UnmarshalJSON(data []byte) error {
	type wire Tool
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = Tool(decoded)
	t.Raw = raw
	return nil
}

func (t Tool) MarshalJSON() ([]byte, error) {
	raw := cloneObject(t.Raw)
	setJSONField(raw, "name", t.Name, false)
	setJSONField(raw, "title", t.Title, true)
	setJSONField(raw, "description", t.Description, true)
	setJSONField(raw, "inputSchema", t.InputSchema, false)
	setJSONField(raw, "outputSchema", t.OutputSchema, true)
	setJSONField(raw, "annotations", t.Annotations, true)
	setJSONField(raw, "icons", t.Icons, true)
	setJSONField(raw, "_meta", t.Meta, true)
	return json.Marshal(raw)
}

// ToolContent covers all MCP tool-result content kinds. Resource contains the
// complete embedded resource object. Raw retains unknown fields for forward
// compatibility.
type ToolContent struct {
	Type        string                   `json:"type"`
	Text        string                   `json:"text,omitempty"`
	Data        string                   `json:"data,omitempty"`
	MIMEType    string                   `json:"mimeType,omitempty"`
	Resource    map[string]interface{}   `json:"resource,omitempty"`
	URI         string                   `json:"uri,omitempty"`
	Name        string                   `json:"name,omitempty"`
	Title       string                   `json:"title,omitempty"`
	Description string                   `json:"description,omitempty"`
	Size        *int64                   `json:"size,omitempty"`
	Annotations map[string]interface{}   `json:"annotations,omitempty"`
	Icons       []map[string]interface{} `json:"icons,omitempty"`
	Meta        map[string]interface{}   `json:"_meta,omitempty"`
	Raw         map[string]interface{}   `json:"-"`
}

func (c *ToolContent) UnmarshalJSON(data []byte) error {
	type wire ToolContent
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = ToolContent(decoded)
	c.Raw = raw
	return nil
}

func (c ToolContent) MarshalJSON() ([]byte, error) {
	raw := cloneObject(c.Raw)
	setJSONField(raw, "type", c.Type, false)
	setJSONField(raw, "text", c.Text, true)
	setJSONField(raw, "data", c.Data, true)
	setJSONField(raw, "mimeType", c.MIMEType, true)
	setJSONField(raw, "resource", c.Resource, true)
	setJSONField(raw, "uri", c.URI, true)
	setJSONField(raw, "name", c.Name, true)
	setJSONField(raw, "title", c.Title, true)
	setJSONField(raw, "description", c.Description, true)
	setJSONField(raw, "size", c.Size, true)
	setJSONField(raw, "annotations", c.Annotations, true)
	setJSONField(raw, "icons", c.Icons, true)
	setJSONField(raw, "_meta", c.Meta, true)
	return json.Marshal(raw)
}

type ToolCallResult struct {
	Content           []ToolContent          `json:"content,omitempty"`
	StructuredContent map[string]interface{} `json:"structuredContent,omitempty"`
	IsError           bool                   `json:"isError,omitempty"`
	Meta              map[string]interface{} `json:"_meta,omitempty"`
	Raw               map[string]interface{} `json:"-"`
}

func (r *ToolCallResult) UnmarshalJSON(data []byte) error {
	type wire ToolCallResult
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = ToolCallResult(decoded)
	r.Raw = raw
	return nil
}

func (r ToolCallResult) MarshalJSON() ([]byte, error) {
	raw := cloneObject(r.Raw)
	setJSONField(raw, "content", r.Content, true)
	setJSONField(raw, "structuredContent", r.StructuredContent, true)
	setJSONField(raw, "isError", r.IsError, true)
	setJSONField(raw, "_meta", r.Meta, true)
	return json.Marshal(raw)
}

func cloneObject(input map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{}, len(input)+8)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func setJSONField(object map[string]interface{}, key string, value interface{}, omitEmpty bool) {
	if omitEmpty && isEmptyJSONValue(value) {
		delete(object, key)
		return
	}
	object[key] = value
}

func isEmptyJSONValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case bool:
		return !typed
	case map[string]interface{}:
		return len(typed) == 0
	case []map[string]interface{}:
		return len(typed) == 0
	case []ToolContent:
		return len(typed) == 0
	case *int64:
		return typed == nil
	default:
		return false
	}
}
