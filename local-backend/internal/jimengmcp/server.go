package jimengmcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Server struct {
	adapter *Adapter
	mux     *http.ServeMux
}

func NewServer(adapter *Adapter) http.Handler {
	if adapter == nil {
		adapter = NewAdapter(AdapterConfig{})
	}
	s := &Server{adapter: adapter, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleRPC)
	return s.mux
}

type serverRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type serverRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *serverRPCError `json:"error,omitempty"`
}

type serverRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type toolCallResponse struct {
	Content           []toolTextContent      `json:"content,omitempty"`
	StructuredContent map[string]interface{} `json:"structuredContent,omitempty"`
	IsError           bool                   `json:"isError"`
}

type toolTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRPC(w, serverRPCResponse{JSONRPC: "2.0", Error: &serverRPCError{Code: -32600, Message: "method not allowed"}})
		return
	}
	var req serverRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, serverRPCResponse{JSONRPC: "2.0", Error: &serverRPCError{Code: -32700, Message: "invalid JSON"}})
		return
	}
	resp := serverRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "tools/list":
		resp.Result = map[string]interface{}{"tools": toolDefinitions()}
	case "tools/call":
		result, err := s.callTool(r, req.Params)
		if err != nil {
			resp.Error = &serverRPCError{Code: -32602, Message: err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &serverRPCError{Code: -32601, Message: "method not found"}
	}
	writeRPC(w, resp)
}

func (s *Server) callTool(r *http.Request, params interface{}) (toolCallResponse, error) {
	var call toolCallParams
	if err := decodeJSON(params, &call); err != nil {
		return toolCallResponse{}, err
	}
	if call.Arguments == nil {
		call.Arguments = map[string]interface{}{}
	}
	switch call.Name {
	case "jimeng.check_status":
		return successResult(map[string]interface{}{"available": true, "command": "dreamina"}), nil
	case "jimeng.inspect_command":
		command, _ := call.Arguments["command"].(string)
		out, err := s.adapter.InspectCommand(r.Context(), command)
		return resultFromMap(out, err), nil
	case "jimeng.login_headless":
		out, err := s.adapter.LoginHeadless(r.Context())
		return resultFromMap(out, err), nil
	case "jimeng.check_login":
		deviceCode, _ := call.Arguments["device_code"].(string)
		poll := intArg(call.Arguments["poll"])
		out, err := s.adapter.CheckLogin(r.Context(), deviceCode, poll)
		return resultFromMap(out, err), nil
	case "jimeng.generate_image":
		var req GenerateImageRequest
		if err := decodeJSON(call.Arguments, &req); err != nil {
			return toolCallResponse{}, err
		}
		out, err := s.adapter.GenerateImage(r.Context(), req)
		return resultFromGeneration(out, err), nil
	case "jimeng.generate_video":
		var req GenerateVideoRequest
		if err := decodeJSON(call.Arguments, &req); err != nil {
			return toolCallResponse{}, err
		}
		out, err := s.adapter.GenerateVideo(r.Context(), req)
		return resultFromGeneration(out, err), nil
	case "jimeng.query_result":
		var req QueryResultRequest
		if err := decodeJSON(call.Arguments, &req); err != nil {
			return toolCallResponse{}, err
		}
		out, err := s.adapter.QueryResult(r.Context(), req)
		return resultFromGeneration(out, err), nil
	case "jimeng.list_task":
		genStatus, _ := call.Arguments["gen_status"].(string)
		out, err := s.adapter.ListTask(r.Context(), genStatus)
		return resultFromMap(out, err), nil
	default:
		return toolCallResponse{}, fmt.Errorf("unknown tool %q", call.Name)
	}
}

func toolDefinitions() []toolDefinition {
	objectSchema := map[string]interface{}{"type": "object"}
	return []toolDefinition{
		{Name: "jimeng.check_status", Description: "Check local Dreamina CLI status", InputSchema: objectSchema},
		{Name: "jimeng.inspect_command", Description: "Read Dreamina CLI help for a command", InputSchema: objectSchema},
		{Name: "jimeng.login_headless", Description: "Start Dreamina headless OAuth login", InputSchema: objectSchema},
		{Name: "jimeng.check_login", Description: "Check Dreamina OAuth login completion", InputSchema: objectSchema},
		{Name: "jimeng.generate_image", Description: "Generate images through user-managed Dreamina CLI", InputSchema: objectSchema},
		{Name: "jimeng.generate_video", Description: "Generate videos through user-managed Dreamina CLI", InputSchema: objectSchema},
		{Name: "jimeng.query_result", Description: "Query or download a Dreamina generation result", InputSchema: objectSchema},
		{Name: "jimeng.list_task", Description: "List Dreamina generation history", InputSchema: objectSchema},
	}
}

func resultFromGeneration(result *GenerationResult, err error) toolCallResponse {
	if err != nil {
		return errorResult(err)
	}
	content := map[string]interface{}{
		"submit_id":  result.SubmitID,
		"gen_status": result.GenStatus,
	}
	if len(result.DownloadedFiles) > 0 {
		content["downloaded_files"] = result.DownloadedFiles
	}
	if result.Raw != nil {
		for k, v := range result.Raw {
			content[k] = v
		}
	}
	return successResult(content)
}

func resultFromMap(result map[string]interface{}, err error) toolCallResponse {
	if err != nil {
		return errorResult(err)
	}
	return successResult(result)
}

func successResult(content map[string]interface{}) toolCallResponse {
	return toolCallResponse{
		Content:           []toolTextContent{{Type: "text", Text: "ok"}},
		StructuredContent: content,
		IsError:           false,
	}
}

func errorResult(err error) toolCallResponse {
	return toolCallResponse{
		Content: []toolTextContent{{Type: "text", Text: err.Error()}},
		IsError: true,
	}
}

func decodeJSON(input interface{}, out interface{}) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func intArg(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		var i int
		_, _ = fmt.Sscanf(v, "%d", &i)
		return i
	default:
		return 0
	}
}

func writeRPC(w http.ResponseWriter, resp serverRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func normalizeToolName(name string) string {
	return strings.TrimSpace(name)
}
