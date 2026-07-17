// Package jsonx provides utilities for extracting and parsing JSON from LLM outputs.
// LLMs often wrap JSON in markdown code fences, add explanatory text, or include
// trailing commas — this package normalizes such output before unmarshaling.
package jsonx

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	// Matches markdown fenced code blocks: ```json ... ``` or ``` ... ```
	mdFenceRe = regexp.MustCompile(`(?s)^` + "```" + `(?:json)?\s*\n?(.*?)\n?` + "```" + `\s*$`)
	// Matches entire string wrapped in ```...``` even with surrounding whitespace
	mdFenceGreedyRe = regexp.MustCompile("(?s)" + "```" + `(?:json)?\s*\n?(.*?)\n?` + "```")
	// Trailing comma before } or ]
	trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)
)

// ExtractJSON extracts valid JSON from raw LLM output text and unmarshals it into v.
// It handles common LLM formatting issues:
//   - Markdown code fences (```json ... ``` or ``` ... ```)
//   - Text before or after the JSON block
//   - Trailing commas in objects and arrays
//   - Unescaped control characters (newlines, tabs) inside JSON strings
func ExtractJSON(raw string, v interface{}) error {
	cleaned := extractJSONBlock(raw)
	if cleaned == "" {
		return fmt.Errorf("no JSON found in LLM output (raw: %.200s)", raw)
	}

	// Try direct parse first
	if err := json.Unmarshal([]byte(cleaned), v); err == nil {
		return nil
	}

	// Try fixing trailing commas
	fixed := trailingCommaRe.ReplaceAllString(cleaned, "$1")
	if err := json.Unmarshal([]byte(fixed), v); err == nil {
		return nil
	}

	// Try normalizing unescaped control characters within strings (LLMs often
	// output literal newlines inside JSON string values)
	normalized := normalizeStringChars(fixed)
	if err := json.Unmarshal([]byte(normalized), v); err != nil {
		return fmt.Errorf("failed to parse LLM JSON: %w (cleaned: %.200s)", err, cleaned)
	}
	return nil
}

// extractJSONBlock attempts to isolate a JSON object or array from raw text.
func extractJSONBlock(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// If the entire string is a markdown fenced block, extract its content
	if m := mdFenceRe.FindStringSubmatch(s); len(m) == 2 {
		inner := strings.TrimSpace(m[1])
		if inner != "" {
			return inner
		}
	}

	// Try to find a markdown fence anywhere in the text and extract the first one
	if m := mdFenceGreedyRe.FindStringSubmatch(s); len(m) == 2 {
		inner := strings.TrimSpace(m[1])
		if inner != "" && (strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "[")) {
			return inner
		}
	}

	// No fence found — try to find balanced { } or [ ] block
	for i, ch := range s {
		if ch == '{' || ch == '[' {
			closeCh := '}'
			if ch == '[' {
				closeCh = ']'
			}
			if extracted := extractBalanced(s[i:], ch, closeCh); extracted != "" {
				return extracted
			}
		}
	}

	// Fallback: return original trimmed string
	return s
}

// normalizeStringChars escapes unescaped control characters (\n, \r, \t) that
// appear inside JSON string values. LLMs often output literal newlines within
// string content, which violates the JSON spec (RFC 7159 requires control
// characters U+0000–U+001F to be escaped).
func normalizeStringChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false

	for _, ch := range s {
		if escaped {
			escaped = false
			b.WriteRune(ch)
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			b.WriteRune(ch)
			continue
		}
		if ch == '"' {
			inString = !inString
			b.WriteRune(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				b.WriteString("\\n")
			case '\r':
				b.WriteString("\\r")
			case '\t':
				b.WriteString("\\t")
			default:
				b.WriteRune(ch)
			}
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// extractBalanced extracts a balanced bracket/brace block from the start of s,
// assuming s[0] is the opening character. Returns empty string if unbalanced.
func extractBalanced(s string, open, close rune) string {
	depth := 0
	inString := false
	escaped := false

	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}
