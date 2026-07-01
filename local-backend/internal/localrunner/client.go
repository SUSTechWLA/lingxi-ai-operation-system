package localrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localtool"
)

type Config struct {
	CloudAPIBase string
	UserToken    string
	DeviceID     string
	RunnerID     string
	SessionID    string
	HTTPClient   *http.Client
}

type Client struct {
	cfg    Config
	client *http.Client
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, client: httpClient}
}

type RegisterRunnerRequest struct {
	DeviceID      string       `json:"deviceId"`
	UserID        string       `json:"userId,omitempty"`
	RunnerVersion string       `json:"runnerVersion"`
	Platform      PlatformInfo `json:"platform"`
	WorkspaceRoot string       `json:"workspaceRoot"`
	Capabilities  []Capability `json:"capabilities"`
}

type RegisterRunnerResponse struct {
	RunnerID             string `json:"runnerId"`
	SessionID            string `json:"sessionId"`
	HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
	PollIntervalSec      int    `json:"pollIntervalSec"`
}

type PlatformInfo struct {
	OS       string `json:"os,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type Capability struct {
	ToolName  string `json:"toolName"`
	Command   string `json:"command"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

type HeartbeatRequest struct {
	SessionID     string  `json:"sessionId"`
	Status        string  `json:"status"`
	RunningJobs   int     `json:"runningJobs"`
	DiskFreeMb    int64   `json:"diskFreeMb"`
	CPULoad       float64 `json:"cpuLoad"`
	MemoryUsageMb int64   `json:"memoryUsageMb"`
}

type ClaimJobResponse struct {
	Job *localtool.Job `json:"job"`
}

type ProgressRequest struct {
	Status   string   `json:"status"`
	Progress float64  `json:"progress"`
	Step     string   `json:"step"`
	Message  string   `json:"message"`
	Logs     []string `json:"logs,omitempty"`
}

type CompleteJobRequest struct {
	Success bool                   `json:"success"`
	Output  map[string]interface{} `json:"output"`
}

type FailJobRequest struct {
	Success     bool                   `json:"success"`
	Error       map[string]interface{} `json:"error"`
	Retryable   bool                   `json:"retryable"`
	Diagnostics map[string]interface{} `json:"diagnostics,omitempty"`
}

func (c *Client) Register(ctx context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	var resp RegisterRunnerResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/local-runners/register", req, &resp); err != nil {
		return nil, err
	}
	c.cfg.RunnerID = resp.RunnerID
	c.cfg.SessionID = resp.SessionID
	return &resp, nil
}

// RunnerID returns the registered runner ID (available after Register succeeds).
func (c *Client) RunnerID() string {
	return c.cfg.RunnerID
}

func (c *Client) Heartbeat(ctx context.Context, runnerID string, req HeartbeatRequest) error {
	if req.SessionID == "" {
		req.SessionID = c.cfg.SessionID
	}
	return c.doJSON(ctx, http.MethodPost, "/api/local-runners/"+runnerID+"/heartbeat", req, nil)
}

func (c *Client) ClaimJob(ctx context.Context, runnerID string) (*localtool.Job, error) {
	var resp ClaimJobResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/local-runners/"+runnerID+"/jobs/claim", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Job, nil
}

func (c *Client) ReportProgress(ctx context.Context, jobID string, req ProgressRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/api/local-jobs/"+jobID+"/progress", req, nil)
}

func (c *Client) CompleteJob(ctx context.Context, jobID string, req CompleteJobRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/api/local-jobs/"+jobID+"/complete", req, nil)
}

func (c *Client) FailJob(ctx context.Context, jobID string, req FailJobRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/api/local-jobs/"+jobID+"/fail", req, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	targetURL, err := c.cloudURL(path)
	if err != nil {
		return err
	}
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.UserToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.UserToken)
	}
	if c.cfg.DeviceID != "" {
		req.Header.Set("X-Device-ID", c.cfg.DeviceID)
	}
	if c.cfg.RunnerID != "" {
		req.Header.Set("X-Runner-ID", c.cfg.RunnerID)
	}
	if c.cfg.SessionID != "" {
		req.Header.Set("X-Runner-Session-ID", c.cfg.SessionID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloud API %s %s returned %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) cloudURL(path string) (string, error) {
	base := strings.TrimRight(c.cfg.CloudAPIBase, "/")
	if base == "" {
		return "", fmt.Errorf("cloud API base is required")
	}
	if strings.HasSuffix(base, "/api") && strings.HasPrefix(path, "/api/") {
		path = strings.TrimPrefix(path, "/api")
	}
	return base + path, nil
}
