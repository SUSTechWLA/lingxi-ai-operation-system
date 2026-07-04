package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FinalReviewExecutor performs quality checks on a rendered video and produces
// a FINAL_REVIEW artifact with metadata.passed indicating whether the video
// meets the production quality standards.
type FinalReviewExecutor struct {
	guard   *PathGuard
	dataDir string
}

// NewFinalReviewExecutor creates a FinalReviewExecutor.
func NewFinalReviewExecutor(guard *PathGuard, dataDir string) *FinalReviewExecutor {
	return &FinalReviewExecutor{guard: guard, dataDir: dataDir}
}

// Execute runs the final review checks.
func (e *FinalReviewExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := job.ProjectID
	if projectID == "" {
		return nil, fmt.Errorf("final_review: projectId is required")
	}

	// Resolve video path
	videoRef, _ := job.Payload["videoRef"].(string)
	probeRef, _ := job.Payload["probeRef"].(string)

	checks := map[string]bool{
		"fileExists":          false,
		"fileSizeValid":       false,
		"durationValid":       false,
		"resolutionValid":     false,
		"hasVideoTrack":       false,
		"hasReadableMetadata": false,
	}
	blockingIssues := []string{}
	warnings := []string{}

	// 1. Check video file exists
	if videoRef != "" {
		localPath, err := e.guard.ResolveLocalURI(videoRef)
		if err == nil {
			info, err := os.Stat(localPath)
			if err == nil {
				checks["fileExists"] = true
				if info.Size() > 0 {
					checks["fileSizeValid"] = true
				} else {
					blockingIssues = append(blockingIssues, "视频文件大小为0")
				}
			} else {
				blockingIssues = append(blockingIssues, "视频文件不存在: "+localPath)
			}
		} else {
			blockingIssues = append(blockingIssues, "无法解析视频路径: "+videoRef)
		}
	} else {
		blockingIssues = append(blockingIssues, "未提供视频引用")
	}

	// 2. Check FFMPEG_PROBE metadata
	if probeRef != "" {
		localPath, err := e.guard.ResolveLocalURI(probeRef)
		if err == nil {
			data, err := os.ReadFile(localPath)
			if err == nil {
				checks["hasReadableMetadata"] = true
				var probe map[string]interface{}
				if json.Unmarshal(data, &probe) == nil {
					// Check from probe structure: expects media.durationSec, media.width, media.height, etc.
					if media, ok := probe["media"].(map[string]interface{}); ok {
						if dur, ok := media["durationSec"].(float64); ok && dur >= 45 && dur <= 90 {
							checks["durationValid"] = true
						} else {
							warnings = append(warnings, "视频时长不符合45-90秒要求")
						}
						if w, ok := media["width"].(float64); ok && w == 1920 {
							if h, ok2 := media["height"].(float64); ok2 && h == 1080 {
								checks["resolutionValid"] = true
							}
						}
						if !checks["resolutionValid"] {
							warnings = append(warnings, "视频分辨率不符合1920x1080要求")
						}
						if codec, ok := media["videoCodec"].(string); ok && codec != "" {
							checks["hasVideoTrack"] = true
						} else {
							blockingIssues = append(blockingIssues, "视频无有效视频轨")
						}
					}
				}
			}
		}
	}

	// Determine pass/fail: all checks must pass, no blocking issues
	passed := checks["fileExists"] && checks["fileSizeValid"] &&
		checks["durationValid"] && checks["resolutionValid"] &&
		checks["hasVideoTrack"] && len(blockingIssues) == 0

	summary := "视频质量检查"
	if passed {
		summary = "视频文件存在，时长、分辨率、视频流、文件大小均符合要求。"
	} else {
		summary = fmt.Sprintf("视频质量检查未通过: %v", blockingIssues)
	}

	// Build the FINAL_REVIEW artifact
	outputRef := fmt.Sprintf("local://projects/%s/reviews/final_review.json", projectID)
	outputDir := filepath.Join(e.dataDir, "projects", projectID, "reviews")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("final_review: create output dir: %w", err)
	}
	reviewData := map[string]interface{}{
		"passed":         passed,
		"summary":        summary,
		"checks":         checks,
		"blockingIssues": blockingIssues,
		"warnings":       warnings,
	}
	reviewBytes, _ := json.MarshalIndent(reviewData, "", "  ")
	outputPath := filepath.Join(outputDir, "final_review.json")
	if err := os.WriteFile(outputPath, reviewBytes, 0644); err != nil {
		return nil, fmt.Errorf("final_review: write review: %w", err)
	}

	output := map[string]interface{}{
		"summary": summary,
		"passed":  passed,
		"checks":  checks,
		"artifacts": []map[string]interface{}{
			{
				"unitId":         "final-review",
				"kind":           "FINAL_REVIEW",
				"name":           "final_review.json",
				"storageType":    "local",
				"storageRef":     outputRef,
				"mimeType":       "application/json",
				"sizeBytes":      len(reviewBytes),
				"status":         "valid",
				"humanApproved":  false,
				"dependsOn":      []string{"VIDEO", "FFMPEG_PROBE_REPORT"},
				"producedByTool": "final_review",
				"producedByRole": "质量审核",
				"metadata": map[string]interface{}{
					"passed":         passed,
					"checks":         checks,
					"blockingIssues": blockingIssues,
					"warnings":       warnings,
				},
			},
		},
	}

	return &Result{Output: output}, nil
}
