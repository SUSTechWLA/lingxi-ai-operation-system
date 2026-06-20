package modelgateway

// Capability defines a model capability category.
type Capability string

const (
	CapTextToText      Capability = "text_to_text"
	CapImageToText     Capability = "image_to_text"
	CapTextToImage     Capability = "text_to_image"
	CapTextToVideo     Capability = "text_to_video"
	CapImageToVideo    Capability = "image_to_video"
)

// ModelRequest represents a unified model call request.
type ModelRequest struct {
	Capability  Capability              `json:"capability"`
	Model       string                  `json:"model,omitempty"`
	Messages    []Message               `json:"messages,omitempty"`
	Images      []ImageInput            `json:"images,omitempty"`
	Parameters  map[string]interface{}  `json:"parameters,omitempty"`
	Fingerprint string                  `json:"fingerprint,omitempty"`
	ProjectID   string                  `json:"projectId,omitempty"`
	Test        *TestConfig             `json:"_test,omitempty"` // fault injection
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ImageInput represents an image for multimodal models.
type ImageInput struct {
	URL    string `json:"url,omitempty"`
	Data   string `json:"data,omitempty"` // base64
	Format string `json:"format,omitempty"`
}

// TestConfig allows fault injection in fake provider.
type TestConfig struct {
	FailMode  string `json:"failMode"`  // rate_limit, timeout, server_error, invalid_schema
	FailCount int    `json:"failCount"`
}

// ModelResult represents a unified model call result.
type ModelResult struct {
	Content     string        `json:"content"`
	Images      []ImageOutput `json:"images,omitempty"`
	Usage       Usage         `json:"usage"`
	Fingerprint string        `json:"fingerprint"`
	Cached      bool          `json:"cached"`
}

// ImageOutput represents a generated image.
type ImageOutput struct {
	URL    string `json:"url,omitempty"`
	Data   string `json:"data,omitempty"`
	Format string `json:"format,omitempty"`
}

// Usage tracks model call usage.
type Usage struct {
	Model        string  `json:"model"`
	PromptTokens int     `json:"promptTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
	DurationMs   int64   `json:"durationMs"`
}

// GatewayError classifies model call errors.
type GatewayError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Retry   bool   `json:"retry"`
}

func (e *GatewayError) Error() string {
	return e.Code + ": " + e.Message
}

// Error codes
const (
	ErrRateLimited      = "PROVIDER_RATE_LIMITED"
	ErrAuthFailed       = "PROVIDER_AUTH_FAILED"
	ErrTimeout          = "PROVIDER_TIMEOUT"
	ErrContentRejected  = "PROVIDER_CONTENT_REJECTED"
	ErrInvalidRequest   = "PROVIDER_INVALID_REQUEST"
	ErrUnavailable      = "PROVIDER_UNAVAILABLE"
	ErrSchemaInvalid    = "OUTPUT_SCHEMA_INVALID"
	ErrDownloadFailed   = "OUTPUT_DOWNLOAD_FAILED"
)
