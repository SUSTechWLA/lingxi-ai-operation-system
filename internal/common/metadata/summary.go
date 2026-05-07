package metadata

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// nodeRefPattern matches {{node_id.output.field}} references in DAG node inputs.
var nodeRefPattern = regexp.MustCompile(`\{\{([^.]+)\.output\.([^}]+)\}\}`)

// BuildInputSummary creates a human-readable summary of tool input parameters.
// Large values are truncated; binary/large fields are summarized with counts.
// Template references like {{node.output.field}} are skipped — data flow is
// already visible from DAG edges and upstream node outputs.
func BuildInputSummary(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	summary := make(map[string]interface{}, len(input))
	for k, v := range input {
		switch val := v.(type) {
		case string:
			if isNodeRef(val) {
				continue
			}
			runes := []rune(val)
			if len(runes) > 200 {
				summary[k] = string(runes[:200]) + "..."
			} else {
				summary[k] = val
			}
		case []interface{}:
			summary[k] = fmt.Sprintf("[%d items]", len(val))
		case map[string]interface{}:
			summary[k] = fmt.Sprintf("{...%d keys}", len(val))
		default:
			if v != nil {
				summary[k] = v
			}
		}
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

func isNodeRef(s string) bool {
	return nodeRefPattern.MatchString(s)
}

// BuildOutputSummary parses tool stdout JSON and extracts human-readable fields.
// Large/binary fields like base64-encoded keyframes are summarized with counts.
func BuildOutputSummary(stdout string) map[string]interface{} {
	if stdout == "" {
		return nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		runes := []rune(stdout)
		if len(runes) > 200 {
			return map[string]interface{}{"raw": string(runes[:200]) + "..."}
		}
		return map[string]interface{}{"raw": stdout}
	}

	summary := make(map[string]interface{})

	// Meaningful fields to extract in priority order (skip large binary fields).
	meaningfulKeys := []string{
		"title", "description", "reply", "keywords", "tags", "platform",
		"duration_sec", "width", "height", "fps", "file_size_mb", "has_audio",
		"audio_duration_sec", "content", "summary", "visual_analysis", "body",
		// Video tool debug fields
		"video_codec", "audio_codec",
		"frame_count", "strategy_used", "extraction_method",
		"frame_resolution", "total_frames_size_mb",
		"transcription_length", "transcription_chars",
		"keyframes_count", "visual_analysis_chars",
		"cached_video_path",
	}

	for _, key := range meaningfulKeys {
		v, ok := parsed[key]
		if !ok || v == nil {
			continue
		}
		switch val := v.(type) {
		case string:
			if val == "" {
				continue
			}
			runes := []rune(val)
			if len(runes) > 300 {
				summary[key] = string(runes[:300]) + "..."
			} else {
				summary[key] = val
			}
		case []interface{}:
			strs := make([]string, 0, len(val))
			for _, item := range val {
				if s, ok := item.(string); ok {
					strs = append(strs, s)
				}
			}
			if len(strs) > 0 {
				summary[key] = strs
			}
		default:
			summary[key] = v
		}
	}

	// Summarize keyframe/image arrays without storing the actual data.
	if keyframes, ok := parsed["keyframes_data_urls"].([]interface{}); ok {
		summary["keyframes_count"] = len(keyframes)
	}
	if imageURLs, ok := parsed["image_urls"].([]interface{}); ok {
		summary["image_urls_count"] = len(imageURLs)
	}

	if len(summary) == 0 {
		return nil
	}
	return summary
}
