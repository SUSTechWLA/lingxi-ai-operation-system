package localmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

type Client struct {
	cfg        ProviderConfig
	httpClient *http.Client
	nextID     atomic.Uint64
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
	if err := c.call(ctx, "tools/list", nil, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("tool name is required")
	}
	var out ToolCallResult
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}
	if params["arguments"] == nil {
		params["arguments"] = map[string]interface{}{}
	}
	if err := c.call(ctx, "tools/call", params, &out); err != nil {
		return nil, err
	}
	if out.StructuredContent == nil {
		out.StructuredContent = map[string]interface{}{}
	}
	return &out, nil
}

func (c *Client) call(ctx context.Context, method string, params interface{}, out interface{}) error {
	if !c.cfg.Enabled {
		return fmt.Errorf("mcp provider %q is disabled", c.cfg.ID)
	}
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
