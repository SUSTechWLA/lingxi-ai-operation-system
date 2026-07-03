package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// VideoFrameQAExecutor extracts representative video frames and runs simple,
// deterministic visual checks for text-zone crowding and over-complex frames.
type VideoFrameQAExecutor struct {
	guard   *PathGuard
	dataDir string
}

func NewVideoFrameQAExecutor(dataDir string) *VideoFrameQAExecutor {
	return &VideoFrameQAExecutor{guard: NewPathGuard(dataDir), dataDir: dataDir}
}

type visualQAThresholds struct {
	TopLeftWarning  float64
	TopLeftBlocking float64
	LowerWarning    float64
	LowerBlocking   float64
	FullWarning     float64
}

type visualQAFrameResult struct {
	FramePath string             `json:"framePath,omitempty"`
	FrameID   string             `json:"frameId,omitempty"`
	ShotID    string             `json:"shotId,omitempty"`
	TimeSec   float64            `json:"timeSec,omitempty"`
	Passed    bool               `json:"passed"`
	Metrics   map[string]float64 `json:"metrics"`
	Issues    []visualQAIssue    `json:"issues,omitempty"`
}

type visualQAIssue struct {
	Code       string  `json:"code"`
	Severity   string  `json:"severity"`
	Zone       string  `json:"zone,omitempty"`
	Message    string  `json:"message"`
	Suggestion string  `json:"suggestion,omitempty"`
	Value      float64 `json:"value,omitempty"`
	Threshold  float64 `json:"threshold,omitempty"`
}

type visualQAShotWindow struct {
	ShotID string
	Start  float64
	End    float64
}

type visualQAShotSummary struct {
	ShotID              string             `json:"shotId"`
	FrameCount          int                `json:"frameCount"`
	SampledTimesSec     []float64          `json:"sampledTimesSec,omitempty"`
	Passed              bool               `json:"passed"`
	NeedsRegeneration   bool               `json:"needsRegeneration"`
	Score               int                `json:"score"`
	BlockingIssueCount  int                `json:"blockingIssueCount"`
	WarningIssueCount   int                `json:"warningIssueCount"`
	MetricSummary       map[string]float64 `json:"metricSummary"`
	Conclusion          string             `json:"conclusion"`
	Recommendations     []string           `json:"recommendations,omitempty"`
	RepresentativeIssue []visualQAIssue    `json:"representativeIssues,omitempty"`
}

func (e *VideoFrameQAExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "projectId")
	}
	if projectID == "" {
		return nil, fmt.Errorf("video_frame_qa: projectId is required")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("video_frame_qa: invalid projectId: %w", err)
	}

	inputURI := firstNonEmptyLocalString(
		stringFromPayload(job.Payload, "input"),
		stringFromPayload(job.Payload, "videoRef"),
		stringFromPayload(job.Payload, "finalVideo"),
	)
	if inputURI == "" {
		return nil, fmt.Errorf("video_frame_qa: input video is required")
	}
	inputPath, err := e.guard.ResolveLocalURI(inputURI)
	if err != nil {
		return nil, fmt.Errorf("video_frame_qa: invalid input path: %w", err)
	}
	if err := e.guard.EnsureReadable(inputURI); err != nil {
		return nil, fmt.Errorf("video_frame_qa: input not readable: %w", err)
	}

	sampleInterval := numberFromPayload(job.Payload, "sampleIntervalSec", 4)
	if sampleInterval <= 0 {
		sampleInterval = 4
	}

	reportDir := filepath.Join(e.dataDir, "projects", projectID, "reports", "video_frame_qa")
	framesDir := filepath.Join(reportDir, "frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return nil, fmt.Errorf("video_frame_qa: create frames dir: %w", err)
	}
	if err := clearFrameDirectory(framesDir); err != nil {
		return nil, fmt.Errorf("video_frame_qa: clear frames dir: %w", err)
	}

	framePattern := filepath.Join(framesDir, "frame_%03d.png")
	filter := fmt.Sprintf("fps=1/%s", formatFloat(sampleInterval))
	if output, err := exec.CommandContext(ctx, "ffmpeg", "-y", "-v", "error", "-i", inputPath, "-vf", filter, framePattern).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("video_frame_qa: extract frames: %w: %s", err, strings.TrimSpace(string(output)))
	}

	framePaths, err := filepath.Glob(filepath.Join(framesDir, "frame_*.png"))
	if err != nil {
		return nil, fmt.Errorf("video_frame_qa: list frames: %w", err)
	}
	sort.Strings(framePaths)
	if len(framePaths) == 0 {
		return nil, fmt.Errorf("video_frame_qa: no frames extracted")
	}

	contactSheetPath := filepath.Join(reportDir, "contact_sheet.jpg")
	_ = exec.CommandContext(ctx,
		"ffmpeg", "-y", "-v", "error", "-i", inputPath,
		"-vf", fmt.Sprintf("fps=1/%s,scale=480:-1,tile=%s", formatFloat(sampleInterval), visualQAContactSheetTile(len(framePaths))),
		"-frames:v", "1",
		contactSheetPath,
	).Run()

	windows := visualQAShotWindowsFromPayload(job.Payload)
	results := make([]visualQAFrameResult, 0, len(framePaths))
	blockingCount := 0
	warningCount := 0
	for i, framePath := range framePaths {
		frame, err := analyzeFrameVisualQuality(framePath, visualQAThresholds{})
		if err != nil {
			return nil, fmt.Errorf("video_frame_qa: analyze %s: %w", filepath.Base(framePath), err)
		}
		timeSec := float64(i) * sampleInterval
		frame.FramePath = "local://projects/" + projectID + "/reports/video_frame_qa/frames/" + filepath.Base(framePath)
		frame.FrameID = fmt.Sprintf("frame_%03d", i+1)
		frame.TimeSec = timeSec
		frame.ShotID = visualQAShotIDAt(windows, timeSec)
		for _, issue := range frame.Issues {
			if issue.Severity == "blocking" {
				blockingCount++
			} else {
				warningCount++
			}
		}
		results = append(results, frame)
	}

	shotSummaries := buildVisualQAShotSummaries(results)
	repairPlan := visualQARepairPlan(shotSummaries)
	passed := blockingCount == 0
	score := 100 - blockingCount*18 - warningCount*5
	if score < 0 {
		score = 0
	}
	report := map[string]interface{}{
		"passed":             passed,
		"score":              score,
		"frameCount":         len(results),
		"blockingIssueCount": blockingCount,
		"warningIssueCount":  warningCount,
		"sampleIntervalSec":  sampleInterval,
		"summary":            visualQASummary(passed, blockingCount, warningCount),
		"frames":             results,
		"shotCount":          len(shotSummaries),
		"shotSummaries":      shotSummaries,
		"needsRegeneration":  repairPlan["needsRegeneration"],
		"repairPlan":         repairPlan,
		"policy": map[string]interface{}{
			"topLeftTextZone": "品牌、镜头标签、源画面文字不能同时挤在左上安全区",
			"lowerThird":      "主标题、字幕、原片字幕不能在底部重复叠加",
			"fullFrame":       "画面复杂度过高时应减少文字、裁切或加深遮罩",
		},
	}
	reportBytes, _ := json.MarshalIndent(report, "", "  ")
	reportPath := filepath.Join(reportDir, "video_frame_qa.json")
	if err := os.WriteFile(reportPath, reportBytes, 0o644); err != nil {
		return nil, fmt.Errorf("video_frame_qa: write report: %w", err)
	}

	artifacts := []map[string]interface{}{
		{
			"unitId":         "video-frame-qa",
			"kind":           "VIDEO_VISUAL_QA_REPORT",
			"name":           "video_frame_qa.json",
			"storageType":    "local",
			"storageRef":     "local://projects/" + projectID + "/reports/video_frame_qa/video_frame_qa.json",
			"mimeType":       "application/json",
			"sizeBytes":      len(reportBytes),
			"status":         "valid",
			"humanApproved":  false,
			"dependsOn":      []string{"VIDEO"},
			"producedByTool": "video_frame_qa",
			"producedByRole": "视觉质量审核",
			"metadata": map[string]interface{}{
				"passed":             passed,
				"score":              score,
				"blockingIssueCount": blockingCount,
				"warningIssueCount":  warningCount,
				"frameCount":         len(results),
				"shotCount":          len(shotSummaries),
				"needsRegeneration":  repairPlan["needsRegeneration"],
			},
		},
	}
	if info, err := os.Stat(contactSheetPath); err == nil && info.Size() > 0 {
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":         "video-frame-qa-contact-sheet",
			"kind":           "VIDEO_VISUAL_QA_CONTACT_SHEET",
			"name":           "contact_sheet.jpg",
			"storageType":    "local",
			"storageRef":     "local://projects/" + projectID + "/reports/video_frame_qa/contact_sheet.jpg",
			"mimeType":       "image/jpeg",
			"sizeBytes":      info.Size(),
			"status":         "valid",
			"humanApproved":  false,
			"dependsOn":      []string{"VIDEO_VISUAL_QA_REPORT"},
			"producedByTool": "video_frame_qa",
			"producedByRole": "视觉质量审核",
		})
	}

	return &Result{Output: map[string]interface{}{
		"success":            true,
		"passed":             passed,
		"score":              score,
		"summary":            report["summary"],
		"reportRef":          "local://projects/" + projectID + "/reports/video_frame_qa/video_frame_qa.json",
		"contactSheetRef":    "local://projects/" + projectID + "/reports/video_frame_qa/contact_sheet.jpg",
		"blockingIssueCount": blockingCount,
		"warningIssueCount":  warningCount,
		"frames":             results,
		"shotCount":          len(shotSummaries),
		"shotSummaries":      shotSummaries,
		"needsRegeneration":  repairPlan["needsRegeneration"],
		"repairPlan":         repairPlan,
		"artifacts":          artifacts,
	}}, nil
}

func analyzeFrameVisualQuality(path string, thresholds visualQAThresholds) (visualQAFrameResult, error) {
	thresholds = thresholds.withDefaults()
	file, err := os.Open(path)
	if err != nil {
		return visualQAFrameResult{}, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return visualQAFrameResult{}, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	zones := map[string]image.Rectangle{
		"topLeft": image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+int(float64(w)*0.42), bounds.Min.Y+int(float64(h)*0.28)),
		"lower":   image.Rect(bounds.Min.X, bounds.Min.Y+int(float64(h)*0.58), bounds.Min.X+w, bounds.Min.Y+int(float64(h)*0.94)),
		"full":    bounds,
	}

	topLeft := frameEdgeDensity(img, zones["topLeft"])
	lower := frameEdgeDensity(img, zones["lower"])
	full := frameEdgeDensity(img, zones["full"])
	metrics := map[string]float64{
		"topLeftTextZoneEdgeDensity": roundMetric(topLeft),
		"lowerThirdEdgeDensity":      roundMetric(lower),
		"fullFrameEdgeDensity":       roundMetric(full),
	}

	issues := []visualQAIssue{}
	if topLeft >= thresholds.TopLeftBlocking {
		issues = append(issues, visualQAIssue{
			Code:       "top_left_text_zone_crowded",
			Severity:   "blocking",
			Zone:       "top_left",
			Message:    "左上文字安全区过于拥挤，容易出现品牌、镜头标签和源画面文字互相覆盖。",
			Suggestion: "该 shot 应减少左上角叠字，隐藏源截图文字，或把标题移到底部单一信息层。",
			Value:      roundMetric(topLeft),
			Threshold:  thresholds.TopLeftBlocking,
		})
	} else if topLeft >= thresholds.TopLeftWarning {
		issues = append(issues, visualQAIssue{
			Code:       "top_left_text_zone_busy",
			Severity:   "warning",
			Zone:       "top_left",
			Message:    "左上文字安全区信息偏多。",
			Suggestion: "建议只保留品牌或镜头标签之一。",
			Value:      roundMetric(topLeft),
			Threshold:  thresholds.TopLeftWarning,
		})
	}
	if lower >= thresholds.LowerBlocking {
		issues = append(issues, visualQAIssue{
			Code:       "lower_third_text_zone_crowded",
			Severity:   "blocking",
			Zone:       "lower_third",
			Message:    "底部三分之一区域过于拥挤，标题、字幕或原片字幕可能互相覆盖。",
			Suggestion: "减少底部文案行数，裁掉原片字幕，或提高底部遮罩并保留单一字幕层。",
			Value:      roundMetric(lower),
			Threshold:  thresholds.LowerBlocking,
		})
	} else if lower >= thresholds.LowerWarning {
		issues = append(issues, visualQAIssue{
			Code:       "lower_third_text_zone_busy",
			Severity:   "warning",
			Zone:       "lower_third",
			Message:    "底部三分之一区域信息密度偏高。",
			Suggestion: "建议压缩字幕行数或缩短标题。",
			Value:      roundMetric(lower),
			Threshold:  thresholds.LowerWarning,
		})
	}
	if full >= thresholds.FullWarning {
		issues = append(issues, visualQAIssue{
			Code:       "frame_visual_clutter_high",
			Severity:   "warning",
			Zone:       "full_frame",
			Message:    "整帧视觉复杂度偏高，可能影响短视频观看理解。",
			Suggestion: "建议降低背景细节、放大主体、减少伪 UI 或伪文字元素。",
			Value:      roundMetric(full),
			Threshold:  thresholds.FullWarning,
		})
	}

	passed := true
	for _, issue := range issues {
		if issue.Severity == "blocking" {
			passed = false
			break
		}
	}
	return visualQAFrameResult{Passed: passed, Metrics: metrics, Issues: issues}, nil
}

func (t visualQAThresholds) withDefaults() visualQAThresholds {
	if t.TopLeftWarning == 0 {
		t.TopLeftWarning = 0.09
	}
	if t.TopLeftBlocking == 0 {
		t.TopLeftBlocking = 0.11
	}
	if t.LowerWarning == 0 {
		t.LowerWarning = 0.075
	}
	if t.LowerBlocking == 0 {
		t.LowerBlocking = 0.095
	}
	if t.FullWarning == 0 {
		t.FullWarning = 0.105
	}
	return t
}

func frameEdgeDensity(img image.Image, rect image.Rectangle) float64 {
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() {
		return 0
	}
	step := 4
	edges := 0
	total := 0
	for y := rect.Min.Y; y+step < rect.Max.Y; y += step {
		for x := rect.Min.X; x+step < rect.Max.X; x += step {
			current := lumaAt(img, x, y)
			right := lumaAt(img, x+step, y)
			down := lumaAt(img, x, y+step)
			if math.Abs(current-right)+math.Abs(current-down) > 55 {
				edges++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(edges) / float64(total)
}

func lumaAt(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

func clearFrameDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "frame_") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func visualQAShotWindowsFromPayload(payload map[string]interface{}) []visualQAShotWindow {
	raw, ok := payload["shotList"].([]interface{})
	if !ok {
		return nil
	}
	windows := make([]visualQAShotWindow, 0, len(raw))
	start := 0.0
	for i, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		shotID := firstNonEmptyLocalString(
			stringFromMapLocal(m, "shotId"),
			stringFromMapLocal(m, "id"),
			fmt.Sprintf("shot_%02d", i+1),
		)
		duration := numberFromMapLocal(m, "durationSec", numberFromMapLocal(m, "duration", 0))
		if duration <= 0 {
			duration = 4
		}
		windows = append(windows, visualQAShotWindow{ShotID: shotID, Start: start, End: start + duration})
		start += duration
	}
	return windows
}

func visualQAShotIDAt(windows []visualQAShotWindow, timeSec float64) string {
	for _, window := range windows {
		if timeSec >= window.Start && timeSec < window.End {
			return window.ShotID
		}
	}
	if len(windows) > 0 && timeSec >= windows[len(windows)-1].End {
		return windows[len(windows)-1].ShotID
	}
	return ""
}

func visualQASummary(passed bool, blockingCount, warningCount int) string {
	if passed && warningCount == 0 {
		return "抽帧视觉 QA 通过：未发现文字安全区拥挤或明显画面复杂度风险。"
	}
	if passed {
		return fmt.Sprintf("抽帧视觉 QA 通过但有 %d 个警告，建议人工复看联系表。", warningCount)
	}
	return fmt.Sprintf("抽帧视觉 QA 未通过：发现 %d 个阻断问题、%d 个警告，需要调整对应 shot 后重新渲染。", blockingCount, warningCount)
}

func buildVisualQAShotSummaries(frames []visualQAFrameResult) []visualQAShotSummary {
	type accumulator struct {
		summary                       visualQAShotSummary
		topLeftSum, lowerSum, fullSum float64
		maxTopLeft, maxLower, maxFull float64
		recommendationSet             map[string]bool
		issueSet                      map[string]bool
	}

	order := []string{}
	byShot := map[string]*accumulator{}
	for _, frame := range frames {
		shotID := strings.TrimSpace(frame.ShotID)
		if shotID == "" {
			shotID = "unmapped_shot"
		}
		acc, ok := byShot[shotID]
		if !ok {
			acc = &accumulator{
				summary: visualQAShotSummary{
					ShotID:        shotID,
					Passed:        true,
					MetricSummary: map[string]float64{},
				},
				recommendationSet: map[string]bool{},
				issueSet:          map[string]bool{},
			}
			byShot[shotID] = acc
			order = append(order, shotID)
		}

		acc.summary.FrameCount++
		acc.summary.SampledTimesSec = append(acc.summary.SampledTimesSec, frame.TimeSec)
		topLeft := frame.Metrics["topLeftTextZoneEdgeDensity"]
		lower := frame.Metrics["lowerThirdEdgeDensity"]
		full := frame.Metrics["fullFrameEdgeDensity"]
		acc.topLeftSum += topLeft
		acc.lowerSum += lower
		acc.fullSum += full
		acc.maxTopLeft = math.Max(acc.maxTopLeft, topLeft)
		acc.maxLower = math.Max(acc.maxLower, lower)
		acc.maxFull = math.Max(acc.maxFull, full)

		for _, issue := range frame.Issues {
			if issue.Severity == "blocking" {
				acc.summary.BlockingIssueCount++
				acc.summary.Passed = false
				acc.summary.NeedsRegeneration = true
			} else {
				acc.summary.WarningIssueCount++
			}
			if issue.Code != "" && !acc.issueSet[issue.Code] {
				acc.summary.RepresentativeIssue = append(acc.summary.RepresentativeIssue, issue)
				acc.issueSet[issue.Code] = true
			}
			recommendation := strings.TrimSpace(issue.Suggestion)
			if recommendation == "" {
				recommendation = visualQARecommendationForIssue(issue)
			}
			if recommendation != "" && !acc.recommendationSet[recommendation] {
				acc.summary.Recommendations = append(acc.summary.Recommendations, recommendation)
				acc.recommendationSet[recommendation] = true
			}
		}
	}

	summaries := make([]visualQAShotSummary, 0, len(order))
	for _, shotID := range order {
		acc := byShot[shotID]
		frameCount := float64(acc.summary.FrameCount)
		acc.summary.MetricSummary = map[string]float64{
			"avgTopLeftTextZoneEdgeDensity": roundMetric(acc.topLeftSum / frameCount),
			"maxTopLeftTextZoneEdgeDensity": roundMetric(acc.maxTopLeft),
			"avgLowerThirdEdgeDensity":      roundMetric(acc.lowerSum / frameCount),
			"maxLowerThirdEdgeDensity":      roundMetric(acc.maxLower),
			"avgFullFrameEdgeDensity":       roundMetric(acc.fullSum / frameCount),
			"maxFullFrameEdgeDensity":       roundMetric(acc.maxFull),
		}
		acc.summary.Score = 100 - acc.summary.BlockingIssueCount*18 - acc.summary.WarningIssueCount*5
		if acc.summary.Score < 0 {
			acc.summary.Score = 0
		}
		acc.summary.Conclusion = visualQAShotConclusion(acc.summary)
		if len(acc.summary.Recommendations) == 0 && !acc.summary.Passed {
			acc.summary.Recommendations = append(acc.summary.Recommendations, "压缩该 shot 的文字层级，降低背景复杂度后重新生成。")
		}
		summaries = append(summaries, acc.summary)
	}
	return summaries
}

func visualQAShotConclusion(summary visualQAShotSummary) string {
	if summary.BlockingIssueCount > 0 {
		return fmt.Sprintf("%s 未通过：发现 %d 个阻断问题，建议返修并重生成该 shot。", summary.ShotID, summary.BlockingIssueCount)
	}
	if summary.WarningIssueCount > 0 {
		return fmt.Sprintf("%s 通过但有 %d 个警告，建议人工复看并微调后再进入最终交付。", summary.ShotID, summary.WarningIssueCount)
	}
	return fmt.Sprintf("%s 通过：抽帧文字安全区和画面复杂度稳定。", summary.ShotID)
}

func visualQARecommendationForIssue(issue visualQAIssue) string {
	switch issue.Code {
	case "top_left_text_zone_crowded", "top_left_text_zone_busy":
		return "减少左上角品牌、镜头编号或源画面文字，只保留一个信息层。"
	case "lower_third_text_zone_crowded", "lower_third_text_zone_busy":
		return "压缩底部字幕和标题行数，避免主标题、字幕和原片字幕叠加。"
	case "frame_visual_clutter_high":
		return "降低背景细节和伪 UI 密度，放大主体并加深遮罩。"
	default:
		return ""
	}
}

func visualQARepairPlan(summaries []visualQAShotSummary) map[string]interface{} {
	regenerate := []string{}
	review := []string{}
	recommendations := []string{}
	seenRecommendation := map[string]bool{}
	for _, summary := range summaries {
		if summary.NeedsRegeneration {
			regenerate = append(regenerate, summary.ShotID)
		} else if summary.WarningIssueCount > 0 {
			review = append(review, summary.ShotID)
		}
		for _, recommendation := range summary.Recommendations {
			if recommendation == "" || seenRecommendation[recommendation] {
				continue
			}
			recommendations = append(recommendations, recommendation)
			seenRecommendation[recommendation] = true
		}
	}

	nextAction := "approve"
	conclusion := "所有 shot 抽帧 QA 通过，可以进入发布文案和交付。"
	if len(regenerate) > 0 {
		nextAction = "regenerate_shots"
		conclusion = fmt.Sprintf("%d 个 shot 需要返修重生成：%s。", len(regenerate), strings.Join(regenerate, ", "))
	} else if len(review) > 0 {
		nextAction = "manual_review"
		conclusion = fmt.Sprintf("%d 个 shot 有警告，建议人工复看：%s。", len(review), strings.Join(review, ", "))
	}

	return map[string]interface{}{
		"needsRegeneration":       len(regenerate) > 0,
		"nextAction":              nextAction,
		"conclusion":              conclusion,
		"regenerateShotIds":       regenerate,
		"manualReviewShotIds":     review,
		"globalRecommendations":   recommendations,
		"regenerateShotCount":     len(regenerate),
		"manualReviewShotCount":   len(review),
		"totalEvaluatedShotCount": len(summaries),
	}
}

func visualQAContactSheetTile(frameCount int) string {
	if frameCount <= 0 {
		return "1x1"
	}
	if frameCount > 16 {
		frameCount = 16
	}

	bestCols := 1
	bestRows := frameCount
	bestWaste := frameCount
	for cols := 1; cols <= 4; cols++ {
		rows := int(math.Ceil(float64(frameCount) / float64(cols)))
		if rows > 4 {
			continue
		}
		waste := rows*cols - frameCount
		if waste < bestWaste ||
			(waste == bestWaste && rows < bestRows) ||
			(waste == bestWaste && rows == bestRows && cols > bestCols) {
			bestCols = cols
			bestRows = rows
			bestWaste = waste
		}
	}
	return fmt.Sprintf("%dx%d", bestCols, bestRows)
}

func numberFromPayload(payload map[string]interface{}, key string, fallback float64) float64 {
	if payload == nil {
		return fallback
	}
	return numberFromMapLocal(payload, key, fallback)
}

func numberFromMapLocal(m map[string]interface{}, key string, fallback float64) float64 {
	switch value := m[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed
		}
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func stringFromMapLocal(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if value, ok := m[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func firstNonEmptyLocalString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatFloat(value float64) string {
	if math.Abs(value-math.Round(value)) < 0.0001 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.3f", value)
}

func roundMetric(value float64) float64 {
	return math.Round(value*10000) / 10000
}
