package localmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	clientImplementationName    = "tangying-local-agent"
	clientImplementationVersion = "0.1.0"
)

type Client struct {
	cfg        ProviderConfig
	httpClient *http.Client
	sdkClient  *mcp.Client

	operationMu sync.Mutex
	sessionMu   sync.Mutex
	session     *mcp.ClientSession
	capture     *wireCapture
}

func NewClient(cfg ProviderConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	capture := newWireCapture()
	return &Client{
		cfg:        cfg,
		httpClient: cloneHTTPClientWithHeaders(httpClient, cfg.Headers),
		sdkClient: mcp.NewClient(
			&mcp.Implementation{Name: clientImplementationName, Version: clientImplementationVersion},
			&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}},
		),
		capture: capture,
	}
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	c.capture.beginListTools()
	callCtx, cancel := c.withProviderTimeout(ctx)
	defer cancel()
	session, err := c.ensureSession(callCtx)
	if err != nil {
		return nil, err
	}
	sdkTools := make([]Tool, 0)
	for remoteTool, listErr := range session.Tools(callCtx, nil) {
		if listErr != nil {
			return nil, fmt.Errorf("mcp provider %q list tools: %w", c.cfg.ID, listErr)
		}
		converted, convertErr := convertTool(remoteTool)
		if convertErr != nil {
			return nil, fmt.Errorf("mcp provider %q decode tool %q: %w", c.cfg.ID, remoteTool.Name, convertErr)
		}
		sdkTools = append(sdkTools, converted)
	}
	rawTools, rawErr := c.capture.decodeTools()
	if rawErr != nil {
		return nil, fmt.Errorf("mcp provider %q decode raw tools: %w", c.cfg.ID, rawErr)
	}
	return c.mergeAndFilterTools(sdkTools, rawTools), nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	c.capture.beginCallTool()
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("tool name is required")
	}
	if !c.toolAllowed(name) {
		return nil, fmt.Errorf("mcp provider %q tool %q is disabled or not enabled", c.cfg.ID, name)
	}
	callCtx, cancel := c.withProviderTimeout(ctx)
	defer cancel()
	session, err := c.ensureSession(callCtx)
	if err != nil {
		return nil, err
	}
	arguments := args
	if arguments == nil {
		arguments = map[string]interface{}{}
	}
	result, err := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      c.remoteToolName(name),
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp provider %q call tool %q: %w", c.cfg.ID, name, err)
	}
	converted, rawErr := c.capture.decodeCallResult()
	if rawErr != nil {
		return nil, fmt.Errorf("mcp provider %q decode raw tool result %q: %w", c.cfg.ID, name, rawErr)
	}
	if converted == nil {
		converted, err = convertToolCallResult(result)
		if err != nil {
			return nil, fmt.Errorf("mcp provider %q decode tool result %q: %w", c.cfg.ID, name, err)
		}
	}
	if converted.StructuredContent == nil {
		converted.StructuredContent = map[string]interface{}{}
	}
	return converted, nil
}

func (c *Client) ensureSession(ctx context.Context) (*mcp.ClientSession, error) {
	if !c.cfg.Enabled {
		return nil, fmt.Errorf("mcp provider %q is disabled", c.cfg.ID)
	}
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.session != nil {
		return c.session, nil
	}
	transport, err := c.newTransport()
	if err != nil {
		return nil, err
	}
	session, err := c.sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp provider %q connect: %w", c.cfg.ID, err)
	}
	c.session = session
	return session, nil
}

func (c *Client) newTransport() (mcp.Transport, error) {
	switch c.transport() {
	case "stdio":
		command := strings.TrimSpace(c.cfg.Command)
		if command == "" {
			return nil, fmt.Errorf("mcp provider %q command is required for stdio transport", c.cfg.ID)
		}
		cmd := exec.Command(command, c.cfg.Args...)
		if workingDir := strings.TrimSpace(c.cfg.WorkingDir); workingDir != "" {
			cmd.Dir = workingDir
		}
		if len(c.cfg.Env) > 0 {
			cmd.Env = mergedEnvironment(os.Environ(), c.cfg.Env)
		}
		return &captureTransport{base: &mcp.CommandTransport{Command: cmd}, capture: c.capture}, nil
	case "http":
		endpoint := strings.TrimSpace(c.cfg.Endpoint)
		if endpoint == "" {
			return nil, fmt.Errorf("mcp provider %q endpoint is empty", c.cfg.ID)
		}
		// Tool calls may be side-effecting. Do not replay POST requests after an
		// ambiguous transport timeout; higher layers can make an explicit,
		// policy-aware retry decision.
		return &captureTransport{base: &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: c.httpClient, MaxRetries: -1}, capture: c.capture}, nil
	default:
		return nil, fmt.Errorf("mcp provider %q has unsupported transport %q", c.cfg.ID, c.cfg.Transport)
	}
}

func (c *Client) Close() error {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.session == nil {
		c.capture.clear()
		return nil
	}
	err := c.session.Close()
	c.session = nil
	c.capture.clear()
	return err
}

func (c *Client) mergeAndFilterTools(sdkTools, rawTools []Tool) []Tool {
	byName := make(map[string][]Tool, len(rawTools))
	for _, rawTool := range rawTools {
		byName[rawTool.Name] = append(byName[rawTool.Name], rawTool)
	}
	tools := make([]Tool, 0, len(sdkTools))
	for _, sdkTool := range sdkTools {
		tool := sdkTool
		if candidates := byName[sdkTool.Name]; len(candidates) > 0 {
			tool = candidates[0]
			byName[sdkTool.Name] = candidates[1:]
		}
		tool.Name = c.logicalToolName(tool.Name)
		if c.toolAllowed(tool.Name) {
			tools = append(tools, tool)
		}
	}
	return tools
}

func (c *Client) withProviderTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.cfg.TimeoutSec <= 0 {
		return ctx, func() {}
	}
	providerTimeout := time.Duration(c.cfg.TimeoutSec) * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= providerTimeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, providerTimeout)
}

func (c *Client) transport() string {
	transport := strings.ToLower(strings.TrimSpace(c.cfg.Transport))
	if transport != "" {
		return transport
	}
	if strings.TrimSpace(c.cfg.Command) != "" {
		return "stdio"
	}
	return "http"
}

func (c *Client) remoteToolName(name string) string {
	name = strings.TrimSpace(name)
	if mapped, ok := c.cfg.ToolNameMap[name]; ok && strings.TrimSpace(mapped) != "" {
		return strings.TrimSpace(mapped)
	}
	prefix := strings.TrimSpace(c.cfg.ToolPrefix)
	if prefix != "" && strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	return name
}

func (c *Client) logicalToolName(name string) string {
	name = strings.TrimSpace(name)
	for logical, remote := range c.cfg.ToolNameMap {
		if strings.TrimSpace(remote) == name && strings.TrimSpace(logical) != "" {
			return strings.TrimSpace(logical)
		}
	}
	prefix := strings.TrimSpace(c.cfg.ToolPrefix)
	if prefix != "" && !strings.HasPrefix(name, prefix) {
		return prefix + name
	}
	return name
}

func (c *Client) toolAllowed(logicalName string) bool {
	logicalName = strings.TrimSpace(logicalName)
	if logicalName == "" {
		return false
	}
	if len(c.cfg.EnabledTools) > 0 && !c.toolNameInList(logicalName, c.cfg.EnabledTools) {
		return false
	}
	return !c.toolNameInList(logicalName, c.cfg.DisabledTools)
}

func (c *Client) toolNameInList(logicalName string, list []string) bool {
	remoteName := c.remoteToolName(logicalName)
	for _, configured := range list {
		configured = strings.TrimSpace(configured)
		if configured == "" {
			continue
		}
		configuredLogical := c.logicalToolName(configured)
		configuredRemote := c.remoteToolName(configured)
		if sameToolName(logicalName, configured) ||
			sameToolName(logicalName, configuredLogical) ||
			sameToolName(remoteName, configured) ||
			sameToolName(remoteName, configuredRemote) {
			return true
		}
	}
	return false
}

func sameToolName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	for key, value := range t.headers {
		if strings.TrimSpace(key) != "" {
			cloned.Header.Set(key, value)
		}
	}
	return t.base.RoundTrip(cloned)
}

func cloneHTTPClientWithHeaders(input *http.Client, headers map[string]string) *http.Client {
	cloned := *input
	base := input.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copyHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		if key = strings.TrimSpace(key); key != "" {
			copyHeaders[key] = value
		}
	}
	cloned.Transport = headerRoundTripper{base: base, headers: copyHeaders}
	return &cloned
}

func mergedEnvironment(base []string, overrides map[string]string) []string {
	positions := make(map[string]int, len(base))
	merged := append([]string(nil), base...)
	for index, entry := range merged {
		if key, _, ok := strings.Cut(entry, "="); ok {
			positions[key] = index
		}
	}
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		entry := key + "=" + value
		if index, ok := positions[key]; ok {
			merged[index] = entry
		} else {
			positions[key] = len(merged)
			merged = append(merged, entry)
		}
	}
	return merged
}

func convertTool(input *mcp.Tool) (Tool, error) {
	var output Tool
	err := remarshal(input, &output)
	return output, err
}

func convertToolCallResult(input *mcp.CallToolResult) (*ToolCallResult, error) {
	if input == nil {
		return &ToolCallResult{StructuredContent: map[string]interface{}{}}, nil
	}
	var output ToolCallResult
	if err := remarshal(input, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func remarshal(input, output interface{}) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, output)
}
