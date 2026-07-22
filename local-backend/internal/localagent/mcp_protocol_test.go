package localagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var defaultAgentMCPTestTools = []string{
	"jimeng.check_status",
	"jimeng.generate_video",
	"jimeng.login_headless",
	"jimeng.check_login",
}

type agentMCPBusinessFixture func(toolName string, arguments map[string]interface{}) map[string]interface{}

func newMCPProtocolTestServer(t *testing.T, fixture agentMCPBusinessFixture, toolNames ...string) *httptest.Server {
	t.Helper()
	if len(toolNames) == 0 {
		toolNames = defaultAgentMCPTestTools
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "localagent-test", Version: "1.0.0"}, nil)
	for _, toolName := range toolNames {
		toolName := toolName
		server.AddTool(&mcp.Tool{Name: toolName, InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments := map[string]interface{}{}
			if len(request.Params.Arguments) > 0 {
				if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
					return nil, err
				}
			}
			payload, err := json.Marshal(fixture(request.Params.Name, arguments))
			if err != nil {
				return nil, err
			}
			var result mcp.CallToolResult
			if err := json.Unmarshal(payload, &result); err != nil {
				return nil, err
			}
			return &result, nil
		})
	}
	return httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
}
