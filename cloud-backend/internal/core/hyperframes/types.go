// Package hyperframes provides an HTTP client for the HyperFrames Render Service,
// replacing the previous exec.Command("npx", "hyperframes", ...) approach.
package hyperframes

// RenderRequest is the payload sent to POST /render.
type RenderRequest struct {
	ProjectDir string `json:"projectDir"`
	Entry      string `json:"entry,omitempty"`
	OutputPath string `json:"outputPath"`
	FPS        int    `json:"fps,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Format     string `json:"format,omitempty"`
	Workers    int    `json:"workers,omitempty"`
	UseGPU     bool   `json:"useGpu,omitempty"`
}

// RenderResult is the response from POST /render.
type RenderResult struct {
	OK         bool   `json:"ok"`
	JobID      string `json:"jobId"`
	OutputPath string `json:"outputPath,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// HealthResult is the response from GET /health.
type HealthResult struct {
	OK           bool              `json:"ok"`
	Service      string            `json:"service"`
	Version      string            `json:"version"`
	Dependencies HealthDeps        `json:"dependencies"`
}

// HealthDeps describes the runtime dependency availability.
type HealthDeps struct {
	Node     string `json:"node"`
	FFmpeg   bool   `json:"ffmpeg"`
	Chromium bool   `json:"chromium"`
	Producer bool   `json:"producer"`
}

// LintRequest is the payload sent to POST /lint.
type LintRequest struct {
	ProjectDir string `json:"projectDir"`
	Entry      string `json:"entry,omitempty"`
}

// LintResult is the response from POST /lint.
type LintResult struct {
	OK         bool         `json:"ok"`
	Errors     []LintMessage `json:"errors"`
	Warnings   []LintMessage `json:"warnings"`
	Entry      string        `json:"entry"`
	DurationMs int64         `json:"durationMs"`
}

// LintMessage is a single lint finding.
type LintMessage struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}
