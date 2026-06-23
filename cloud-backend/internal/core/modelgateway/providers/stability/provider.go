package stability

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

// Provider implements modelgateway.Provider for Stability AI REST API (Stable Diffusion 3).
type Provider struct {
	apiKey string
	client *http.Client
}

const defaultStabilityBaseURL = "https://api.stability.ai"

// NewProvider creates a new Stability AI image generation provider.
// Reads STABILITY_API_KEY from environment.
func NewProvider() *Provider {
	return &Provider{
		apiKey: os.Getenv("STABILITY_API_KEY"),
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *Provider) Name() string { return "stability" }

func (p *Provider) Supports(cap modelgateway.Capability) bool {
	return cap == modelgateway.CapTextToImage
}

func (p *Provider) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultStabilityBaseURL+"/v1/user/account", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("stability health check returned %d", resp.StatusCode)
	}
	return nil
}

func (p *Provider) Execute(ctx context.Context, req *modelgateway.ModelRequest) (*modelgateway.ModelResult, error) {
	if p.apiKey == "" {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrAuthFailed,
			Message: "STABILITY_API_KEY is not set",
			Retry:   false,
		}
	}

	prompt, _ := req.Parameters["prompt"].(string)
	if prompt == "" {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: "prompt is required",
			Retry:   false,
		}
	}

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	_ = writer.WriteField("prompt", prompt)
	_ = writer.WriteField("output_format", "png")

	aspectRatio := stringParam(req.Parameters, "aspect_ratio", "16:9")
	_ = writer.WriteField("aspect_ratio", aspectRatio)

	if negPrompt, ok := req.Parameters["negative_prompt"].(string); ok && negPrompt != "" {
		_ = writer.WriteField("negative_prompt", negPrompt)
	}
	if seed, ok := req.Parameters["seed"]; ok {
		_ = writer.WriteField("seed", fmt.Sprintf("%v", seed))
	}

	if err := writer.Close(); err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("failed to build multipart form: %v", err),
			Retry:   false,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultStabilityBaseURL+"/v2beta/stable-image/generate/sd3", &buf)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("failed to create request: %v", err),
			Retry:   true,
		}
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Accept", "image/*")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrTimeout,
			Message: fmt.Sprintf("request failed: %v", err),
			Retry:   true,
		}
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("failed to read response: %v", err),
			Retry:   true,
		}
	}

	if resp.StatusCode >= 400 {
		return nil, classifyStabilityError(resp.StatusCode, string(respBytes))
	}

	// Stability returns image bytes directly on success (content-type image/png)
	ct := resp.Header.Get("Content-Type")
	if len(respBytes) == 0 {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: "empty response body",
			Retry:   true,
		}
	}

	var images []modelgateway.ImageOutput
	if isImageContentType(ct) {
		images = append(images, modelgateway.ImageOutput{
			Data:   base64.StdEncoding.EncodeToString(respBytes),
			Format: "png",
		})
	} else {
		// Try JSON error/finish reason
		var finishResp struct {
			FinishReason string `json:"finish_reason"`
			Errors       []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if json.Unmarshal(respBytes, &finishResp) == nil {
			if finishResp.FinishReason == "CONTENT_FILTERED" {
				return nil, &modelgateway.GatewayError{
					Code:    modelgateway.ErrContentRejected,
					Message: "content filtered by safety system",
					Retry:   false,
				}
			}
			if len(finishResp.Errors) > 0 {
				return nil, &modelgateway.GatewayError{
					Code:    modelgateway.ErrInvalidRequest,
					Message: finishResp.Errors[0].Message,
					Retry:   false,
				}
			}
		}
		// Fallback: include as raw data
		images = append(images, modelgateway.ImageOutput{
			Data: base64.StdEncoding.EncodeToString(respBytes),
		})
	}

	return &modelgateway.ModelResult{
		Images: images,
		Usage: modelgateway.Usage{
			Model: "stable-diffusion-3",
		},
	}, nil
}

func classifyStabilityError(statusCode int, body string) *modelgateway.GatewayError {
	switch statusCode {
	case 401, 403:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrAuthFailed,
			Message: fmt.Sprintf("authentication failed (%d): %s", statusCode, body),
			Retry:   false,
		}
	case 429:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrRateLimited,
			Message: fmt.Sprintf("rate limited (%d): %s", statusCode, body),
			Retry:   true,
		}
	case 400, 422:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("invalid request (%d): %s", statusCode, body),
			Retry:   false,
		}
	default:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("server error (%d): %s", statusCode, body),
			Retry:   true,
		}
	}
}

func isImageContentType(ct string) bool {
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType = ct
	}
	return mediaType == "image/png" || mediaType == "image/jpeg" || mediaType == "image/webp"
}

func stringParam(params map[string]interface{}, key, defaultVal string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}
