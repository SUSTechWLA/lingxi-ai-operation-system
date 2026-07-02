package localagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

const (
	jimengProviderID         = "jimeng"
	defaultJiMengMCPEndpoint = "http://127.0.0.1:18180"
	dreaminaInstallCommand   = "curl -fsSL https://jimeng.jianying.com/cli | bash"
	dreaminaInstallScriptURL = "https://jimeng.jianying.com/cli"
	dreaminaLogDirectory     = "~/.dreamina_cli/logs"
)

type CommandOutput struct {
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (CommandOutput, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) (CommandOutput, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return CommandOutput{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

type LocalMCPProviderSettingsResponse struct {
	Providers []localmcp.ProviderConfig `json:"providers"`
}

type LocalMCPProviderStatusResponse struct {
	Providers []LocalMCPProviderStatus `json:"providers"`
}

type LocalMCPProviderStatus struct {
	localmcp.ProviderConfig
	Reachable bool            `json:"reachable"`
	Error     string          `json:"error,omitempty"`
	Tools     []localmcp.Tool `json:"tools,omitempty"`
}

type JiMengSetupStatusResponse struct {
	DreaminaAvailable  bool                     `json:"dreaminaAvailable"`
	DreaminaVersion    string                   `json:"dreaminaVersion,omitempty"`
	InstallCommand     string                   `json:"installCommand"`
	InstallScriptURL   string                   `json:"installScriptUrl"`
	LogDir             string                   `json:"logDir"`
	MCPProvider        *localmcp.ProviderConfig `json:"mcpProvider,omitempty"`
	MCPProviders       []LocalMCPProviderStatus `json:"mcpProviders,omitempty"`
	DefaultMCPEndpoint string                   `json:"defaultMcpEndpoint"`
	MCPStartCommand    string                   `json:"mcpStartCommand"`
}

type installCLIRequest struct {
	Confirm bool `json:"confirm"`
}

type registerMCPRequest struct {
	Endpoint string `json:"endpoint"`
}

type checkLoginRequest struct {
	DeviceCode string `json:"device_code"`
	Poll       int    `json:"poll"`
}

func (s *Server) handleMCPProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		providers, err := s.readMCPProviders()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, LocalMCPProviderSettingsResponse{Providers: providers})
	case http.MethodPut:
		var req LocalMCPProviderSettingsResponse
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid MCP provider payload")
			return
		}
		providers, err := normalizeMCPProviders(req.Providers)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.writeMCPProviders(providers); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, LocalMCPProviderSettingsResponse{Providers: providers})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleMCPProviderStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	status, err := s.mcpProviderStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LocalMCPProviderStatusResponse{Providers: status})
}

func (s *Server) handleJiMengSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	providers, err := s.readMCPProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := s.mcpProviderStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := JiMengSetupStatusResponse{
		InstallCommand:     dreaminaInstallCommand,
		InstallScriptURL:   dreaminaInstallScriptURL,
		LogDir:             dreaminaLogDirectory,
		MCPProviders:       status,
		DefaultMCPEndpoint: defaultJiMengMCPEndpoint,
		MCPStartCommand:    fmt.Sprintf("jimeng-mcp -addr %s", strings.TrimPrefix(defaultJiMengMCPEndpoint, "http://")),
	}
	for _, provider := range providers {
		if provider.ID == jimengProviderID {
			copy := provider
			resp.MCPProvider = &copy
			break
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out, runErr := s.commandRunner().Run(ctx, "dreamina", "version")
	if runErr == nil {
		resp.DreaminaAvailable = true
		resp.DreaminaVersion = strings.TrimSpace(out.Stdout)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleJiMengInstallCLI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req installCLIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid install payload")
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "explicit confirmation is required before installing Dreamina CLI")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	name, args := installShellCommand()
	out, err := s.commandRunner().Run(ctx, name, args...)
	resp := map[string]interface{}{
		"command":          dreaminaInstallCommand,
		"installScriptUrl": dreaminaInstallScriptURL,
		"stdout":           out.Stdout,
		"stderr":           out.Stderr,
	}
	if err != nil {
		resp["status"] = "failed"
		resp["error"] = err.Error()
		writeJSON(w, http.StatusBadGateway, resp)
		return
	}
	resp["status"] = "ok"
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleJiMengRegisterMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req registerMCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid register payload")
		return
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		endpoint = defaultJiMengMCPEndpoint
	}
	providers, err := s.readMCPProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	provider := localmcp.ProviderConfig{ID: jimengProviderID, Label: "JiMeng MCP", Endpoint: endpoint, Enabled: true}
	providers = upsertMCPProvider(providers, provider)
	if err := s.writeMCPProviders(providers); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "ok",
		"provider":        provider,
		"mcpStartCommand": fmt.Sprintf("jimeng-mcp -addr %s", strings.TrimPrefix(endpoint, "http://")),
	})
}

func (s *Server) handleJiMengLoginHeadless(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := s.callJiMengMCPTool(r.Context(), "jimeng.login_headless", map[string]interface{}{})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleJiMengCheckLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req checkLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid check login payload")
		return
	}
	if strings.TrimSpace(req.DeviceCode) == "" {
		writeError(w, http.StatusBadRequest, "device_code is required")
		return
	}
	if req.Poll <= 0 {
		req.Poll = 30
	}
	result, err := s.callJiMengMCPTool(r.Context(), "jimeng.check_login", map[string]interface{}{
		"device_code": req.DeviceCode,
		"poll":        req.Poll,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) mcpProviderStatus(ctx context.Context) ([]LocalMCPProviderStatus, error) {
	providers, err := s.readMCPProviders()
	if err != nil {
		return nil, err
	}
	statuses := make([]LocalMCPProviderStatus, 0, len(providers))
	for _, provider := range providers {
		status := LocalMCPProviderStatus{ProviderConfig: provider}
		if provider.Enabled {
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			tools, err := localmcp.NewClient(provider, nil).ListTools(checkCtx)
			cancel()
			if err != nil {
				status.Error = err.Error()
			} else {
				status.Reachable = true
				status.Tools = tools
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *Server) callJiMengMCPTool(ctx context.Context, toolName string, args map[string]interface{}) (*localmcp.ToolCallResult, error) {
	providers, err := s.readMCPProviders()
	if err != nil {
		return nil, err
	}
	for _, provider := range providers {
		if provider.ID == jimengProviderID && provider.Enabled {
			callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			return localmcp.NewClient(provider, nil).CallTool(callCtx, toolName, args)
		}
	}
	return nil, errors.New("JiMeng MCP provider is not registered")
}

func (s *Server) readMCPProviders() ([]localmcp.ProviderConfig, error) {
	data, err := os.ReadFile(s.mcpProvidersPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []localmcp.ProviderConfig{}, nil
		}
		return nil, err
	}
	var resp LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return normalizeMCPProviders(resp.Providers)
}

func (s *Server) ReadMCPProviders() ([]localmcp.ProviderConfig, error) {
	return s.readMCPProviders()
}

func (s *Server) writeMCPProviders(providers []localmcp.ProviderConfig) error {
	if err := s.EnsureDirs(); err != nil {
		return err
	}
	normalized, err := normalizeMCPProviders(providers)
	if err != nil {
		return err
	}
	return writeIndentedJSON(s.mcpProvidersPath(), LocalMCPProviderSettingsResponse{Providers: normalized})
}

func (s *Server) mcpProvidersPath() string {
	return filepath.Join(s.paths.ConfigDir, "mcp-providers.json")
}

func (s *Server) commandRunner() CommandRunner {
	if s.cfg.CommandRunner != nil {
		return s.cfg.CommandRunner
	}
	return execCommandRunner{}
}

func normalizeMCPProviders(providers []localmcp.ProviderConfig) ([]localmcp.ProviderConfig, error) {
	out := make([]localmcp.ProviderConfig, 0, len(providers))
	seen := map[string]bool{}
	for _, provider := range providers {
		provider.ID = strings.TrimSpace(provider.ID)
		provider.Label = strings.TrimSpace(provider.Label)
		provider.Endpoint = strings.TrimSpace(provider.Endpoint)
		if provider.ID == "" {
			return nil, errors.New("provider id is required")
		}
		if seen[provider.ID] {
			return nil, fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		if provider.Endpoint == "" {
			return nil, fmt.Errorf("provider %q endpoint is required", provider.ID)
		}
		if provider.Label == "" {
			provider.Label = provider.ID
		}
		seen[provider.ID] = true
		out = append(out, provider)
	}
	return out, nil
}

func upsertMCPProvider(providers []localmcp.ProviderConfig, next localmcp.ProviderConfig) []localmcp.ProviderConfig {
	for idx, provider := range providers {
		if provider.ID == next.ID {
			providers[idx] = next
			return providers
		}
	}
	return append(providers, next)
}

func installShellCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-NoProfile", "-Command", "iwr -useb " + dreaminaInstallScriptURL + " | iex"}
	}
	return "sh", []string{"-c", dreaminaInstallCommand}
}
