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
	jimengToolPrefix         = "jimeng."
	ipAvatarProviderID       = "ip_avatar_3d"
	ipAvatarToolPrefix       = "ip_avatar_3d."
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
	Endpoint      string            `json:"endpoint"`
	Transport     string            `json:"transport"`
	Command       string            `json:"command"`
	Args          []string          `json:"args"`
	Env           map[string]string `json:"env"`
	WorkingDir    string            `json:"workingDir"`
	ToolPrefix    string            `json:"toolPrefix"`
	ToolNameMap   map[string]string `json:"toolNameMap"`
	EnabledTools  []string          `json:"enabledTools"`
	DisabledTools []string          `json:"disabledTools"`
	TimeoutSec    int               `json:"timeout"`
	ApprovalMode  string            `json:"approvalMode"`
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
	defaultCommand, defaultArgs := defaultJiMengStdioCommand()
	resp := JiMengSetupStatusResponse{
		InstallCommand:   dreaminaInstallCommand,
		InstallScriptURL: dreaminaInstallScriptURL,
		LogDir:           dreaminaLogDirectory,
		MCPProviders:     status,
		MCPStartCommand:  strings.TrimSpace(defaultCommand + " " + strings.Join(defaultArgs, " ")),
	}
	for _, provider := range providers {
		if provider.ID == jimengProviderID {
			copy := provider
			resp.MCPProvider = &copy
			resp.MCPStartCommand = mcpStartCommand(provider)
			break
		}
	}
	if resp.MCPProvider != nil {
		checkCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		result, err := s.callJiMengMCPTool(checkCtx, "jimeng.check_status", map[string]interface{}{})
		cancel()
		if err == nil && result != nil {
			resp.DreaminaAvailable = boolFromMap(result.StructuredContent, "available", "loggedIn")
			resp.DreaminaVersion = stringFromMap(result.StructuredContent, "version", "dreaminaVersion")
		}
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
	transport := strings.ToLower(strings.TrimSpace(req.Transport))
	endpoint := strings.TrimSpace(req.Endpoint)
	command := strings.TrimSpace(req.Command)
	args := append([]string(nil), req.Args...)
	if transport == "" {
		if command != "" {
			transport = "stdio"
		} else if endpoint != "" {
			transport = "http"
		} else {
			transport = "stdio"
		}
	}
	if transport == "stdio" && command == "" {
		command, args = defaultJiMengStdioCommand()
	}
	toolPrefix := strings.TrimSpace(req.ToolPrefix)
	if transport == "stdio" && toolPrefix == "" {
		toolPrefix = jimengToolPrefix
	}
	providers, err := s.readMCPProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	provider := localmcp.ProviderConfig{
		ID:            jimengProviderID,
		Label:         "JiMeng MCP",
		Endpoint:      endpoint,
		Transport:     transport,
		Command:       command,
		Args:          args,
		Env:           req.Env,
		WorkingDir:    req.WorkingDir,
		ToolPrefix:    toolPrefix,
		ToolNameMap:   req.ToolNameMap,
		EnabledTools:  req.EnabledTools,
		DisabledTools: req.DisabledTools,
		TimeoutSec:    req.TimeoutSec,
		ApprovalMode:  req.ApprovalMode,
		Enabled:       true,
	}
	normalized, err := normalizeMCPProviders([]localmcp.ProviderConfig{provider})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	provider = normalized[0]
	providers = upsertMCPProvider(providers, provider)
	if err := s.writeMCPProviders(providers); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "ok",
		"provider":        provider,
		"mcpStartCommand": mcpStartCommand(provider),
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
			client := localmcp.NewClient(provider, nil)
			tools, err := client.ListTools(checkCtx)
			_ = client.Close()
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
			client := localmcp.NewClient(provider, nil)
			defer client.Close()
			return client.CallTool(callCtx, toolName, args)
		}
	}
	return nil, errors.New("JiMeng MCP provider is not registered")
}

func (s *Server) readMCPProviders() ([]localmcp.ProviderConfig, error) {
	data, err := os.ReadFile(s.mcpProvidersPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return withBundledIPAvatarProvider(nil)
		}
		return nil, err
	}
	var resp LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return withBundledIPAvatarProvider(resp.Providers)
}

func withBundledIPAvatarProvider(providers []localmcp.ProviderConfig) ([]localmcp.ProviderConfig, error) {
	normalized, err := normalizeMCPProviders(providers)
	if err != nil {
		return nil, err
	}
	for _, provider := range normalized {
		if provider.ID == ipAvatarProviderID {
			return normalized, nil
		}
	}
	if provider, ok := defaultIPAvatarProvider(); ok {
		normalized = append(normalized, provider)
	}
	return normalized, nil
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
		provider.Transport = strings.ToLower(strings.TrimSpace(provider.Transport))
		provider.Command = strings.TrimSpace(provider.Command)
		provider.WorkingDir = strings.TrimSpace(provider.WorkingDir)
		provider.ToolPrefix = strings.TrimSpace(provider.ToolPrefix)
		provider.ApprovalMode = strings.TrimSpace(provider.ApprovalMode)
		provider.Args = compactArgs(provider.Args)
		provider.Env = compactEnv(provider.Env)
		provider.ToolNameMap = compactToolNameMap(provider.ToolNameMap)
		provider.EnabledTools = compactStringList(provider.EnabledTools)
		provider.DisabledTools = compactStringList(provider.DisabledTools)
		if provider.ID == "" {
			return nil, errors.New("provider id is required")
		}
		if seen[provider.ID] {
			return nil, fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		if provider.Transport == "" {
			if provider.Command != "" {
				provider.Transport = "stdio"
			} else {
				provider.Transport = "http"
			}
		}
		switch provider.Transport {
		case "http":
			if provider.Endpoint == "" {
				return nil, fmt.Errorf("provider %q endpoint is required", provider.ID)
			}
		case "stdio":
			if provider.Command == "" {
				return nil, fmt.Errorf("provider %q command is required for stdio transport", provider.ID)
			}
			provider.Endpoint = ""
		default:
			return nil, fmt.Errorf("provider %q transport %q is unsupported", provider.ID, provider.Transport)
		}
		if provider.Endpoint == "" && provider.Transport == "http" {
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

func compactArgs(args []string) []string {
	return compactStringList(args)
}

func compactStringList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func compactEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compactToolNameMap(toolNameMap map[string]string) map[string]string {
	if len(toolNameMap) == 0 {
		return nil
	}
	out := make(map[string]string, len(toolNameMap))
	for logical, remote := range toolNameMap {
		logical = strings.TrimSpace(logical)
		remote = strings.TrimSpace(remote)
		if logical == "" || remote == "" {
			continue
		}
		out[logical] = remote
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func boolFromMap(input map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if value, ok := input[key]; ok {
			if boolValue, ok := value.(bool); ok {
				return boolValue
			}
			if stringValue, ok := value.(string); ok {
				switch strings.ToLower(strings.TrimSpace(stringValue)) {
				case "true", "yes", "ok", "available", "authenticated":
					return true
				}
			}
		}
	}
	return false
}

func stringFromMap(input map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key]; ok {
			if stringValue, ok := value.(string); ok {
				return strings.TrimSpace(stringValue)
			}
		}
	}
	return ""
}

func mcpStartCommand(provider localmcp.ProviderConfig) string {
	if provider.Transport == "stdio" {
		return strings.TrimSpace(provider.Command + " " + strings.Join(provider.Args, " "))
	}
	return strings.TrimSpace(provider.Endpoint)
}

func defaultJiMengStdioCommand() (string, []string) {
	command := strings.TrimSpace(os.Getenv("JIMENG_MCP_COMMAND"))
	if command == "" {
		command = detectPython3ForMCP()
	}
	return command, []string{defaultJiMengStdioScriptPath()}
}

func defaultIPAvatarProvider() (localmcp.ProviderConfig, bool) {
	script := defaultIPAvatarStdioScriptPath()
	if strings.TrimSpace(script) == "" {
		return localmcp.ProviderConfig{}, false
	}
	if info, err := os.Stat(script); err != nil || info.IsDir() {
		return localmcp.ProviderConfig{}, false
	}
	workingDir := filepath.Dir(filepath.Dir(filepath.Dir(script)))
	return localmcp.ProviderConfig{
		ID:           ipAvatarProviderID,
		Label:        "Tangying 3D IP Avatar",
		Transport:    "stdio",
		Command:      detectPython3ForMCP(),
		Args:         []string{script},
		WorkingDir:   workingDir,
		ToolPrefix:   ipAvatarToolPrefix,
		TimeoutSec:   3600,
		ApprovalMode: "before_execute",
		Enabled:      true,
	}, true
}

func detectPython3ForMCP() string {
	for _, candidate := range pythonCandidatesForMCP() {
		command := compatiblePythonExecutable(candidate)
		if command != "" {
			return command
		}
	}
	return "python3"
}

func pythonCandidatesForMCP() []string {
	candidates := []string{}
	if path, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if matches, err := filepath.Glob(filepath.Join(home, ".pyenv", "versions", "*", "bin", "python3")); err == nil {
			candidates = append(candidates, matches...)
		}
	}
	candidates = append(candidates, "/opt/homebrew/bin/python3", "/usr/local/bin/python3", "python3")
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

func compatiblePythonExecutable(candidate string) string {
	out, err := exec.Command(candidate, "-c", "import sys; sys.exit(1) if sys.version_info < (3, 10) else print(sys.executable)").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func defaultJiMengStdioScriptPath() string {
	if script := strings.TrimSpace(os.Getenv("JIMENG_MCP_STDIO_SCRIPT")); script != "" {
		return script
	}
	cwd, err := os.Getwd()
	if err == nil {
		candidates := []string{
			filepath.Join(cwd, "mcp", "jimeng", "server.py"),
			filepath.Join(cwd, "..", "mcp", "jimeng", "server.py"),
			filepath.Join(cwd, "..", "..", "mcp", "jimeng", "server.py"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				if abs, err := filepath.Abs(candidate); err == nil {
					return abs
				}
				return candidate
			}
		}
	}
	return filepath.Join("mcp", "jimeng", "server.py")
}

func defaultIPAvatarStdioScriptPath() string {
	if script := strings.TrimSpace(os.Getenv("TANGYING_IP_AVATAR_MCP_SCRIPT")); script != "" {
		if absolute, err := filepath.Abs(script); err == nil {
			return absolute
		}
		return script
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for _, candidate := range []string{
		filepath.Join(cwd, "mcp", "ip_avatar_3d", "server.py"),
		filepath.Join(cwd, "..", "mcp", "ip_avatar_3d", "server.py"),
		filepath.Join(cwd, "..", "..", "mcp", "ip_avatar_3d", "server.py"),
		filepath.Join(cwd, "..", "..", "..", "mcp", "ip_avatar_3d", "server.py"),
	} {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			if absolute, absErr := filepath.Abs(candidate); absErr == nil {
				return absolute
			}
			return candidate
		}
	}
	return ""
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
