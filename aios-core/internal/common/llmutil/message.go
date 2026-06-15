// Package llmutil provides helpers for building LLM API messages,
// particularly for multimodal (vision) requests.
package llmutil

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// ImageURL represents an image reference for multimodal LLM requests.
// URL can be an HTTP(S) URL, or a base64 data URL (data:image/...;base64,...).
type ImageURL struct {
	URL string `json:"url"`
}

// ContentPart is a single part in a multimodal message content array.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// BuildUserMessage creates a user message for the OpenAI-compatible API.
//
// When imageURLs is empty, it returns a simple text message:
//
//	{"role": "user", "content": "text"}
//
// When imageURLs are provided, it returns a multimodal content array:
//
//	{"role": "user", "content": [
//	  {"type": "text", "text": "..."},
//	  {"type": "image_url", "image_url": {"url": "..."}},
//	  ...
//	]}
//
// Supports both HTTP URLs and base64 data URLs.
func BuildUserMessage(text string, imageURLs []string) map[string]interface{} {
	if len(imageURLs) == 0 {
		return map[string]interface{}{
			"role":    "user",
			"content": text,
		}
	}

	parts := make([]ContentPart, 0, len(imageURLs)+1)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, ContentPart{
			Type: "text",
			Text: text,
		})
	}
	for _, u := range imageURLs {
		parts = append(parts, ContentPart{
			Type: "image_url",
			ImageURL: &ImageURL{URL: u},
		})
	}

	return map[string]interface{}{
		"role":    "user",
		"content": parts,
	}
}

// Base64DataURL formats raw image bytes and a MIME type as a base64 data URL
// suitable for passing to multimodal LLM APIs.
func Base64DataURL(mimeType string, data []byte) string {
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

// IsImageMimeType returns true if the MIME type represents an image format
// supported by multimodal LLMs (JPEG, PNG, WebP, GIF).
func IsImageMimeType(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}
