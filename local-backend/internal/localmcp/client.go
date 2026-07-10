package localmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	cfg        ProviderConfig
	httpClient *http.Client
	nextID     atomic.Uint64
	stdioMu    sync.Mutex
	stdio      *stdioSession
}

func NewClient(cfg ProviderConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{cfg: cfg, httpClient: httpClient}
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var out struct {
		Tools []Tool `json:"tools"`
	}
	callCtx, cancel := c.withProviderTimeout(ctx)
	defer cancel()
	if err := c.call(callCtx, "tools/list", nil, &out); err != nil {
		return nil, err
	}
	filtered := out.Tools[:0]
	for idx := range out.Tools {
		out.Tools[idx].Name = c.logicalToolName(out.Tools[idx].Name)
		if c.toolAllowed(out.Tools[idx].Name) {
			filtered = append(filtered, out.Tools[idx])
		}
	}
	return filtered, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("tool name is required")
	}
	if !c.toolAllowed(name) {
		return nil, fmt.Errorf("mcp provider %q tool %q is disabled or not enabled", c.cfg.ID, name)
	}
	var out ToolCallResult
	params := map[string]interface{}{
		"name":      c.remoteToolName(name),
		"arguments": args,
	}
	if params["arguments"] == nil {
		params["arguments"] = map[string]interface{}{}
	}
	callCtx, cancel := c.withProviderTimeout(ctx)
	defer cancel()
	if err := c.call(callCtx, "tools/call", params, &out); err != nil {
		return nil, err
	}
	if out.StructuredContent == nil {
		out.StructuredContent = map[string]interface{}{}
	}
	return &out, nil
}

func (c *Client) withProviderTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.cfg.TimeoutSec <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSec)*time.Second)
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
	if c.toolNameInList(logicalName, c.cfg.DisabledTools) {
		return false
	}
	return true
}

func (c *Client) toolNameInList(logicalName string, list []string) bool {
	if len(list) == 0 {
		return false
	}
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

func (c *Client) Close() error {
	c.stdioMu.Lock()
	defer c.stdioMu.Unlock()
	if c.stdio == nil {
		return nil
	}
	err := c.stdio.close()
	c.stdio = nil
	return err
}

func (c *Client) call(ctx context.Context, method string, params interface{}, out interface{}) error {
	if !c.cfg.Enabled {
		return fmt.Errorf("mcp provider %q is disabled", c.cfg.ID)
	}
	switch c.transport() {
	case "http":
		return c.callHTTP(ctx, method, params, out)
	case "stdio":
		return c.callStdio(ctx, method, params, out)
	default:
		return fmt.Errorf("mcp provider %q has unsupported transport %q", c.cfg.ID, c.cfg.Transport)
	}
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

func (c *Client) callHTTP(ctx context.Context, method string, params interface{}, out interface{}) error {
	endpoint := strings.TrimSpace(c.cfg.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("mcp provider %q endpoint is empty", c.cfg.ID)
	}
	reqBody := rpcRequest{
		JSONRPC: "2.0",
		ID:      fmt.Sprintf("%d", c.nextID.Add(1)),
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp provider %q returned http %d", c.cfg.ID, resp.StatusCode)
	}
	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return err
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("mcp provider %q rpc error %d: %s", c.cfg.ID, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	payload, err := json.Marshal(rpcResp.Result)
	if err != nil {
		return err
	}
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func (c *Client) callStdio(ctx context.Context, method string, params interface{}, out interface{}) error {
	session, err := c.ensureStdioSession(ctx)
	if err != nil {
		return err
	}
	result, err := session.call(ctx, method, params, fmt.Sprintf("%d", c.nextID.Add(1)))
	if err != nil {
		return err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func (c *Client) ensureStdioSession(ctx context.Context) (*stdioSession, error) {
	c.stdioMu.Lock()
	defer c.stdioMu.Unlock()
	if c.stdio != nil {
		return c.stdio, nil
	}
	command := strings.TrimSpace(c.cfg.Command)
	if command == "" {
		return nil, fmt.Errorf("mcp provider %q command is required for stdio transport", c.cfg.ID)
	}
	session, err := startStdioSession(c.cfg)
	if err != nil {
		return nil, err
	}
	if _, err := session.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "tangying-local-agent",
			"version": "0.1.0",
		},
	}, fmt.Sprintf("%d", c.nextID.Add(1))); err != nil {
		_ = session.close()
		return nil, err
	}
	if err := session.notify(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}); err != nil {
		_ = session.close()
		return nil, err
	}
	c.stdio = session
	return session, nil
}

type stdioSession struct {
	cfg     ProviderConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	encoder *json.Encoder
	decoder *json.Decoder
	stderr  *bytes.Buffer
	mu      sync.Mutex
}

func startStdioSession(cfg ProviderConfig) (*stdioSession, error) {
	cmd := exec.Command(strings.TrimSpace(cfg.Command), cfg.Args...)
	if strings.TrimSpace(cfg.WorkingDir) != "" {
		cmd.Dir = strings.TrimSpace(cfg.WorkingDir)
	}
	if len(cfg.Env) > 0 {
		env := os.Environ()
		for key, value := range cfg.Env {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	return &stdioSession{
		cfg:     cfg,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		encoder: json.NewEncoder(stdin),
		decoder: json.NewDecoder(stdout),
		stderr:  stderr,
	}, nil
}

func (s *stdioSession) call(ctx context.Context, method string, params interface{}, id string) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := s.encoder.Encode(req); err != nil {
		return nil, s.withStderr(err)
	}
	type decodeResult struct {
		resp rpcResponse
		err  error
	}
	for {
		ch := make(chan decodeResult, 1)
		go func() {
			var resp rpcResponse
			err := s.decoder.Decode(&resp)
			ch <- decodeResult{resp: resp, err: err}
		}()
		select {
		case <-ctx.Done():
			_ = s.close()
			return nil, ctx.Err()
		case result := <-ch:
			if result.err != nil {
				return nil, s.withStderr(result.err)
			}
			if result.resp.ID != id {
				continue
			}
			if result.resp.Error != nil {
				return nil, fmt.Errorf("mcp provider %q rpc error %d: %s", s.cfg.ID, result.resp.Error.Code, result.resp.Error.Message)
			}
			return result.resp.Result, nil
		}
	}
}

func (s *stdioSession) notify(message map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.encoder.Encode(message); err != nil {
		return s.withStderr(err)
	}
	return nil
}

func (s *stdioSession) close() error {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.stdout != nil {
		_ = s.stdout.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		return s.cmd.Wait()
	}
	return nil
}

func (s *stdioSession) withStderr(err error) error {
	if err == nil {
		return nil
	}
	stderr := strings.TrimSpace(s.stderr.String())
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, stderr)
}
