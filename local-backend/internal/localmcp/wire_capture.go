package localmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// wireCapture observes the raw JSON-RPC result carried by the official SDK
// transport. It never creates or sends protocol messages.
type wireCapture struct {
	mu         sync.Mutex
	pending    map[string]string
	listPages  []json.RawMessage
	callResult json.RawMessage
}

func newWireCapture() *wireCapture {
	return &wireCapture{pending: map[string]string{}}
}

func (c *wireCapture) beginListTools() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = map[string]string{}
	c.listPages = nil
	c.callResult = nil
}

func (c *wireCapture) beginCallTool() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = map[string]string{}
	c.listPages = nil
	c.callResult = nil
}

func (c *wireCapture) recordWrite(message jsonrpc.Message) string {
	request, ok := message.(*jsonrpc.Request)
	if !ok || !request.ID.IsValid() || (request.Method != "tools/list" && request.Method != "tools/call") {
		return ""
	}
	key := jsonRPCIDKey(request.ID)
	c.mu.Lock()
	c.pending[key] = request.Method
	c.mu.Unlock()
	return key
}

func (c *wireCapture) forget(key string) {
	if key == "" {
		return
	}
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

func (c *wireCapture) recordRead(message jsonrpc.Message) {
	response, ok := message.(*jsonrpc.Response)
	if !ok || !response.ID.IsValid() {
		return
	}
	key := jsonRPCIDKey(response.ID)
	c.mu.Lock()
	defer c.mu.Unlock()
	method, ok := c.pending[key]
	if !ok {
		return
	}
	delete(c.pending, key)
	if response.Error != nil || len(response.Result) == 0 {
		return
	}
	raw := append(json.RawMessage(nil), response.Result...)
	switch method {
	case "tools/list":
		c.listPages = append(c.listPages, raw)
	case "tools/call":
		c.callResult = raw
	}
}

func (c *wireCapture) decodeTools() ([]Tool, error) {
	c.mu.Lock()
	pages := make([]json.RawMessage, len(c.listPages))
	for index := range c.listPages {
		pages[index] = append(json.RawMessage(nil), c.listPages[index]...)
	}
	c.listPages = nil
	c.mu.Unlock()
	tools := make([]Tool, 0)
	for _, page := range pages {
		var payload struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(page, &payload); err != nil {
			return nil, err
		}
		for _, rawTool := range payload.Tools {
			var tool Tool
			if err := json.Unmarshal(rawTool, &tool); err != nil {
				return nil, err
			}
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func (c *wireCapture) decodeCallResult() (*ToolCallResult, error) {
	c.mu.Lock()
	raw := append(json.RawMessage(nil), c.callResult...)
	c.callResult = nil
	c.mu.Unlock()
	if len(raw) == 0 {
		return nil, nil
	}
	var result ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *wireCapture) clear() {
	c.mu.Lock()
	c.pending = map[string]string{}
	c.listPages = nil
	c.callResult = nil
	c.mu.Unlock()
}

func jsonRPCIDKey(id jsonrpc.ID) string {
	return fmt.Sprintf("%T:%v", id.Raw(), id.Raw())
}

type captureTransport struct {
	base    mcp.Transport
	capture *wireCapture
}

func (t *captureTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := t.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &captureConnection{base: connection, capture: t.capture}, nil
}

type captureConnection struct {
	base    mcp.Connection
	capture *wireCapture
}

func (c *captureConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.base.Read(ctx)
	if err == nil {
		c.capture.recordRead(message)
	}
	return message, err
}

func (c *captureConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	key := c.capture.recordWrite(message)
	if err := c.base.Write(ctx, message); err != nil {
		c.capture.forget(key)
		return err
	}
	return nil
}

func (c *captureConnection) Close() error      { return c.base.Close() }
func (c *captureConnection) SessionID() string { return c.base.SessionID() }
