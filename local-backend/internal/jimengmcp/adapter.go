package jimengmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type CommandOutput struct {
	Stdout string
	Stderr string
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (CommandOutput, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (CommandOutput, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return CommandOutput{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

type AdapterConfig struct {
	Runner      CommandRunner
	CommandName string
}

type Adapter struct {
	runner      CommandRunner
	commandName string
}

func NewAdapter(cfg AdapterConfig) *Adapter {
	runner := cfg.Runner
	if runner == nil {
		runner = execRunner{}
	}
	commandName := strings.TrimSpace(cfg.CommandName)
	if commandName == "" {
		commandName = "dreamina"
	}
	return &Adapter{runner: runner, commandName: commandName}
}

type GenerateImageRequest struct {
	Mode           string   `json:"mode,omitempty"`
	Prompt         string   `json:"prompt"`
	Images         []string `json:"images,omitempty"`
	Ratio          string   `json:"ratio,omitempty"`
	ResolutionType string   `json:"resolution_type,omitempty"`
	ModelVersion   string   `json:"model_version,omitempty"`
	GenerateNum    int      `json:"generate_num,omitempty"`
	PollSeconds    int      `json:"poll,omitempty"`
}

type GenerateVideoRequest struct {
	Mode            string   `json:"mode,omitempty"`
	Prompt          string   `json:"prompt"`
	Image           string   `json:"image,omitempty"`
	Images          []string `json:"images,omitempty"`
	Video           string   `json:"video,omitempty"`
	Audio           string   `json:"audio,omitempty"`
	Duration        int      `json:"duration,omitempty"`
	Ratio           string   `json:"ratio,omitempty"`
	VideoResolution string   `json:"video_resolution,omitempty"`
	ModelVersion    string   `json:"model_version,omitempty"`
	PollSeconds     int      `json:"poll,omitempty"`
}

type QueryResultRequest struct {
	SubmitID    string `json:"submit_id"`
	DownloadDir string `json:"download_dir,omitempty"`
}

type GenerationResult struct {
	SubmitID        string                 `json:"submit_id"`
	GenStatus       string                 `json:"gen_status"`
	FailReason      string                 `json:"fail_reason,omitempty"`
	DownloadedFiles []string               `json:"downloaded_files,omitempty"`
	Raw             map[string]interface{} `json:"raw,omitempty"`
}

func (a *Adapter) GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerationResult, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "text2image"
	}
	args := []string{mode}
	if len(req.Images) > 0 {
		args = append(args, "--images="+strings.Join(req.Images, ","))
	}
	args = appendIfPresent(args, "prompt", req.Prompt)
	args = appendIfPresent(args, "ratio", req.Ratio)
	args = appendIfPresent(args, "resolution_type", req.ResolutionType)
	args = appendIfPresent(args, "model_version", req.ModelVersion)
	args = appendIfPositive(args, "generate_num", req.GenerateNum)
	args = appendIfPositive(args, "poll", req.PollSeconds)
	return a.runGeneration(ctx, args)
}

func (a *Adapter) GenerateVideo(ctx context.Context, req GenerateVideoRequest) (*GenerationResult, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "text2video"
	}
	args := []string{mode}
	args = appendIfPresent(args, "image", req.Image)
	if len(req.Images) > 0 {
		args = append(args, "--images="+strings.Join(req.Images, ","))
	}
	args = appendIfPresent(args, "video", req.Video)
	args = appendIfPresent(args, "audio", req.Audio)
	args = appendIfPresent(args, "prompt", req.Prompt)
	args = appendIfPositive(args, "duration", req.Duration)
	args = appendIfPresent(args, "ratio", req.Ratio)
	args = appendIfPresent(args, "video_resolution", req.VideoResolution)
	args = appendIfPresent(args, "model_version", req.ModelVersion)
	args = appendIfPositive(args, "poll", req.PollSeconds)
	return a.runGeneration(ctx, args)
}

func (a *Adapter) QueryResult(ctx context.Context, req QueryResultRequest) (*GenerationResult, error) {
	if strings.TrimSpace(req.SubmitID) == "" {
		return nil, errors.New("submit_id is required")
	}
	args := []string{"query_result", "--submit_id=" + req.SubmitID}
	args = appendIfPresent(args, "download_dir", req.DownloadDir)
	return a.runGeneration(ctx, args)
}

func (a *Adapter) LoginHeadless(ctx context.Context) (map[string]interface{}, error) {
	out, err := a.runner.Run(ctx, a.commandName, "login", "--headless")
	if err != nil {
		return nil, commandError(err, out)
	}
	return parseObjectOutput(out.Stdout)
}

func (a *Adapter) CheckLogin(ctx context.Context, deviceCode string, pollSeconds int) (map[string]interface{}, error) {
	if strings.TrimSpace(deviceCode) == "" {
		return nil, errors.New("device_code is required")
	}
	args := []string{"login", "checklogin", "--device_code=" + deviceCode}
	args = appendIfPositive(args, "poll", pollSeconds)
	out, err := a.runner.Run(ctx, a.commandName, args...)
	if err != nil {
		return nil, commandError(err, out)
	}
	return parseObjectOutput(out.Stdout)
}

func (a *Adapter) ListTask(ctx context.Context, genStatus string) (map[string]interface{}, error) {
	args := []string{"list_task"}
	args = appendIfPresent(args, "gen_status", genStatus)
	out, err := a.runner.Run(ctx, a.commandName, args...)
	if err != nil {
		return nil, commandError(err, out)
	}
	return parseObjectOutput(out.Stdout)
}

func (a *Adapter) UserCredit(ctx context.Context) (map[string]interface{}, error) {
	out, err := a.runner.Run(ctx, a.commandName, "user_credit")
	if err != nil {
		return nil, commandError(err, out)
	}
	return parseObjectOutput(out.Stdout)
}

func (a *Adapter) InspectCommand(ctx context.Context, command string) (map[string]interface{}, error) {
	args := []string{"-h"}
	if strings.TrimSpace(command) != "" {
		args = []string{command, "-h"}
	}
	out, err := a.runner.Run(ctx, a.commandName, args...)
	if err != nil {
		return nil, commandError(err, out)
	}
	return map[string]interface{}{"command": command, "help": out.Stdout}, nil
}

func (a *Adapter) runGeneration(ctx context.Context, args []string) (*GenerationResult, error) {
	out, err := a.runner.Run(ctx, a.commandName, args...)
	if err != nil {
		return nil, commandError(err, out)
	}
	return parseGenerationOutput(out.Stdout)
}

func parseGenerationOutput(stdout string) (*GenerationResult, error) {
	obj, err := parseObjectOutput(stdout)
	if err != nil {
		return nil, err
	}
	submitID := firstString(obj, "submit_id", "submitId")
	status := firstString(obj, "gen_status", "genStatus", "status")
	failReason := firstString(obj, "fail_reason", "failReason", "message", "error")
	if status == "fail" || status == "failed" || status == "error" {
		if failReason == "" {
			failReason = "dreamina generation failed"
		}
		return nil, fmt.Errorf("dreamina generation failed: %s", failReason)
	}
	if submitID == "" {
		return nil, errors.New("dreamina output missing submit_id")
	}
	if status != "querying" && status != "success" {
		return nil, fmt.Errorf("dreamina output has unsupported gen_status %q", status)
	}
	return &GenerationResult{
		SubmitID:        submitID,
		GenStatus:       status,
		FailReason:      failReason,
		DownloadedFiles: stringSlice(obj["downloaded_files"]),
		Raw:             obj,
	}, nil
}

func parseObjectOutput(stdout string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, errors.New("dreamina output is empty")
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end < start {
		if obj := parseKeyValueOutput(trimmed); len(obj) > 0 {
			return obj, nil
		}
		return nil, fmt.Errorf("dreamina output is not JSON: %s", firstLine(trimmed))
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func parseKeyValueOutput(value string) map[string]interface{} {
	result := map[string]interface{}{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sep := strings.Index(line, ":")
		if sep < 0 {
			sep = strings.Index(line, "=")
		}
		if sep <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		val := strings.TrimSpace(line[sep+1:])
		if key == "" || val == "" {
			continue
		}
		result[key] = val
	}
	return result
}

func commandError(err error, out CommandOutput) error {
	detail := strings.TrimSpace(out.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(out.Stdout)
	}
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, detail)
}

func appendIfPresent(args []string, name string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return args
	}
	return append(args, "--"+name+"="+value)
}

func appendIfPositive(args []string, name string, value int) []string {
	if value <= 0 {
		return args
	}
	return append(args, "--"+name+"="+strconv.Itoa(value))
}

func firstString(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringSlice(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func firstLine(value string) string {
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return value[:idx]
	}
	return value
}
