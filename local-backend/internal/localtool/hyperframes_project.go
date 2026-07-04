package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// HyperFramesProjectExecutor generates a HyperFrames HTML project from a
// VideoCompositionSpec or falls back to a simple demo layout.
type HyperFramesProjectExecutor struct {
	dataDir string
}

func NewHyperFramesProjectExecutor(dataDir string) *HyperFramesProjectExecutor {
	return &HyperFramesProjectExecutor{dataDir: dataDir}
}

// Execute generates the HyperFrames project directory. If the payload contains
// a compositionSpec, it generates a full card-based layout with overlay and
// caption tracks. Otherwise it falls back to a simple demo HTML page.
func (e *HyperFramesProjectExecutor) Execute(_ context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "projectId")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("invalid project id: %w", err)
	}

	projectRoot := filepath.Join(e.dataDir, "projects", projectID, "hyperframes")
	if err := ensureInside(filepath.Join(e.dataDir, "projects"), projectRoot); err != nil {
		return nil, err
	}

	assetsDir := filepath.Join(projectRoot, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return nil, err
	}

	topic := stringFromPayload(job.Payload, "topic")
	script := stringFromPayload(job.Payload, "script")

	// Check for compositionSpec — the new OneClick Video protocol.
	compSpec := compositionSpecFromPayload(job.Payload)

	var files []string
	localRef := "local://projects/" + projectID + "/hyperframes"

	if compSpec != nil {
		// Full card-based rendering from VideoCompositionSpec.
		if err := e.writeCompositionProject(projectRoot, assetsDir, projectID, topic, compSpec); err != nil {
			return nil, err
		}
		files = []string{
			localRef + "/index.html",
			localRef + "/assets/data.json",
			localRef + "/assets/style.css",
			localRef + "/manifest.json",
			localRef + "/DESIGN.md",
		}
	} else {
		// Legacy demo fallback.
		data := map[string]interface{}{
			"topic":             topic,
			"script":            script,
			"shotList":          job.Payload["shotList"],
			"videoPrompts":      job.Payload["videoPrompts"],
			"shotAssetPackages": job.Payload["shotAssetPackages"],
			"style":             job.Payload["style"],
			"publishCopy":       job.Payload["publishCopy"],
		}
		dataJSON, _ := json.MarshalIndent(data, "", "  ")
		if err := os.WriteFile(filepath.Join(assetsDir, "data.json"), dataJSON, 0o644); err != nil {
			return nil, err
		}

		if err := os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte(buildCompositionStyle()), 0o644); err != nil {
			return nil, err
		}
		mediaPackages := shotMediaPackagesFromPayload(e.dataDir, projectID, projectRoot, job.Payload)
		fallbackSpec := fallbackCompositionSpec()
		var index string
		if len(mediaPackages) > 0 {
			if shotSpec := compositionSpecFromShotListPayload(topic, script, job.Payload); shotSpec != nil {
				fallbackSpec = shotSpec
				mediaPackages = alignMediaPackagesToComposition(mediaPackages, shotSpec)
				index = buildHyperFramesIndexWithMediaSpec(topic, script, mediaPackages, shotSpec)
			} else {
				if duration := totalMediaDuration(mediaPackages); duration > 0 {
					fallbackSpec.DurationSec = duration
				}
				index = buildHyperFramesIndexWithMedia(topic, script, mediaPackages)
			}
		} else if shotSpec := compositionSpecFromShotListPayload(topic, script, job.Payload); shotSpec != nil {
			fallbackSpec = shotSpec
			cards, captions := extractCardsAndCaptions(shotSpec)
			index = buildCompositionIndex(topic, shotSpec, cards, captions, shotSpec.Style)
		} else {
			index = buildHyperFramesIndex(topic, script)
		}
		if err := os.WriteFile(filepath.Join(projectRoot, "index.html"), []byte(index), 0o644); err != nil {
			return nil, err
		}

		manifest := map[string]interface{}{
			"entry":     "index.html",
			"data":      "assets/data.json",
			"projectId": projectID,
			"tool":      "hyperframes_project_generator",
		}
		manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(filepath.Join(projectRoot, "manifest.json"), manifestJSON, 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(projectRoot, "DESIGN.md"), []byte(buildDesignDoc(projectID, topic, fallbackSpec)), 0o644); err != nil {
			return nil, err
		}

		files = []string{
			localRef + "/index.html",
			localRef + "/assets/data.json",
			localRef + "/assets/style.css",
			localRef + "/manifest.json",
			localRef + "/DESIGN.md",
		}
	}

	return &Result{Output: map[string]interface{}{
		"projectDir": localRef,
		"entry":      "index.html",
		"files":      files,
		"summary":    "HyperFrames project generated locally",
		"artifacts": []map[string]interface{}{
			{
				"unitId":         "hyperframes-project",
				"kind":           "HYPERFRAMES_PROJECT",
				"name":           "hyperframes_project",
				"storageType":    "local",
				"storageRef":     "local://projects/" + projectID + "/hyperframes",
				"mimeType":       "text/html",
				"sizeBytes":      0,
				"status":         "valid",
				"humanApproved":  false,
				"dependsOn":      []string{"VIDEO_COMPOSITION_SPEC"},
				"producedByTool": "hyperframes_project_generator",
				"producedByRole": "渲染制片",
				"metadata": map[string]interface{}{
					"entry":      "index.html",
					"fileCount":  len(files),
					"projectDir": localRef,
				},
			},
		},
	}}, nil
}

// writeCompositionProject generates the full card-based HyperFrames project
// from a VideoCompositionSpec.
func (e *HyperFramesProjectExecutor) writeCompositionProject(projectRoot, assetsDir, projectID, topic string, spec *compositionSpec) error {
	// 1. Write assets/data.json with the full spec and derived card data.
	cards, captions := extractCardsAndCaptions(spec)
	style := spec.Style
	if style == nil {
		style = map[string]interface{}{}
	}
	if _, ok := style["theme"]; !ok {
		style["theme"] = "clean_card"
	}
	if _, ok := style["background"]; !ok {
		style["background"] = "soft_gradient"
	}

	data := map[string]interface{}{
		"compositionSpec": spec,
		"cards":           cards,
		"captions":        captions,
		"style":           style,
		"projectId":       projectID,
		"topic":           topic,
		"generatedAt":     time.Now().UTC().Format(time.RFC3339),
	}
	dataJSON, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(filepath.Join(assetsDir, "data.json"), dataJSON, 0o644); err != nil {
		return fmt.Errorf("write data.json: %w", err)
	}

	// 2. Write assets/style.css.
	css := buildCompositionStyle()
	if err := os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte(css), 0o644); err != nil {
		return fmt.Errorf("write style.css: %w", err)
	}

	// 3. Write index.html with card and caption layers.
	htmlContent := buildCompositionIndex(topic, spec, cards, captions, style)
	if err := os.WriteFile(filepath.Join(projectRoot, "index.html"), []byte(htmlContent), 0o644); err != nil {
		return fmt.Errorf("write index.html: %w", err)
	}

	// 4. Write manifest.json.
	manifest := map[string]interface{}{
		"entry":       "index.html",
		"data":        "assets/data.json",
		"style":       "assets/style.css",
		"projectId":   projectID,
		"tool":        "hyperframes_project_generator",
		"specVersion": spec.SpecVersion,
		"projectType": spec.ProjectType,
		"durationSec": spec.DurationSec,
		"fps":         spec.FPS,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(projectRoot, "manifest.json"), manifestJSON, 0o644); err != nil {
		return fmt.Errorf("write manifest.json: %w", err)
	}

	// 5. Write DESIGN.md.
	designDoc := buildDesignDoc(projectID, topic, spec)
	if err := os.WriteFile(filepath.Join(projectRoot, "DESIGN.md"), []byte(designDoc), 0o644); err != nil {
		return fmt.Errorf("write DESIGN.md: %w", err)
	}

	return nil
}

// ---- VideoCompositionSpec types ----

type compositionSpec struct {
	SpecVersion string                   `json:"specVersion"`
	ProjectType string                   `json:"projectType,omitempty"`
	DurationSec float64                  `json:"durationSec"`
	FPS         float64                  `json:"fps"`
	Resolution  map[string]interface{}   `json:"resolution,omitempty"`
	Tracks      []map[string]interface{} `json:"tracks"`
	Style       map[string]interface{}   `json:"style"`
}

type cardInfo struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	StartSec  float64 `json:"startSec"`
	EndSec    float64 `json:"endSec"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Animation string  `json:"animation"`
}

type captionInfo struct {
	ID       string  `json:"id"`
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Text     string  `json:"text"`
}

type shotMediaPackage struct {
	ShotID      string
	DurationSec float64
	Mode        string
	BaseLayer   mediaLayer
	Overlays    []mediaOverlay
	StartSec    float64
}

type mediaLayer struct {
	Kind        string
	StorageRef  string
	ResolvedSrc string
	Missing     bool
}

type mediaOverlay struct {
	ID   string
	Kind string
	Role string
	Text string
}

// compositionSpecFromPayload extracts a VideoCompositionSpec from the job payload.
func compositionSpecFromPayload(payload map[string]interface{}) *compositionSpec {
	raw, ok := payload["compositionSpec"]
	if !ok {
		return nil
	}

	// The compositionSpec might be a string (JSON) or a parsed map.
	var specMap map[string]interface{}
	switch v := raw.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &specMap); err != nil {
			return nil
		}
	case map[string]interface{}:
		specMap = v
	default:
		return nil
	}

	// Re-marshal to parse into our struct.
	b, err := json.Marshal(specMap)
	if err != nil {
		return nil
	}
	var spec compositionSpec
	if err := json.Unmarshal(b, &spec); err != nil {
		return nil
	}
	if spec.SpecVersion == "" {
		return nil
	}
	return &spec
}

func compositionSpecFromShotListPayload(topic, script string, payload map[string]interface{}) *compositionSpec {
	items := shotListItemsFromPayload(payload)
	if len(items) == 0 {
		return nil
	}
	var overlayItems []interface{}
	var captionItems []interface{}
	nextStart := 0.0
	for index, item := range items {
		shot := mapFromInterface(item)
		if shot == nil {
			continue
		}
		shotID := firstStringFromMap(shot, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", index+1)
		}
		duration := floatFromMap(shot, "durationSec", 0)
		rawStart, hasStart := floatValueFromMap(shot, "startSec")
		rawEnd, hasEnd := floatValueFromMap(shot, "endSec")
		start := nextStart
		if hasStart && rawStart >= nextStart {
			start = rawStart
		}
		if duration <= 0 && hasStart && hasEnd && rawEnd > rawStart {
			duration = rawEnd - rawStart
		}
		if duration <= 0 {
			duration = 6
		}
		end := start + duration
		nextStart = end

		title := firstStringFromMap(shot, "sceneSummary", "visual", "description", "title", "mainAction")
		if title == "" {
			title = fmt.Sprintf("画面 %02d", index+1)
		}
		body := firstStringFromMap(shot, "mainAction", "action", "camera", "composition", "plannedAssetRoute")
		if body == "" {
			body = firstStringFromMap(shot, "description", "narrationText", "scriptText")
		}
		if body == "" && index == 0 {
			body = script
		}
		caption := firstStringFromMap(shot, "narrationText", "scriptText", "voiceoverText", "subtitleText", "text")
		route := strings.ToLower(firstStringFromMap(shot, "plannedAssetRoute", "assetRoute", "route", "recommendedMode"))
		kind := "content_card"
		switch {
		case strings.Contains(route, "screen"):
			kind = "screen_recording_card"
		case strings.Contains(route, "aigc"):
			kind = "aigc_card"
		case strings.Contains(route, "hyperframes"):
			kind = "motion_card"
		}

		overlayItems = append(overlayItems, map[string]interface{}{
			"id":        "card-" + shotID,
			"kind":      kind,
			"startSec":  start,
			"endSec":    end,
			"title":     title,
			"body":      body,
			"animation": "slide_up",
		})
		if caption != "" {
			captionItems = append(captionItems, map[string]interface{}{
				"id":       "caption-" + shotID,
				"startSec": start,
				"endSec":   end,
				"text":     caption,
			})
		}
	}
	if len(overlayItems) == 0 {
		return nil
	}
	if strings.TrimSpace(topic) == "" {
		topic = "Tangying AIOS Video"
	}
	duration := nextStart
	if duration <= 0 {
		duration = 8
	}
	return &compositionSpec{
		SpecVersion: "oneclick-video/v1",
		ProjectType: "image_text_video",
		DurationSec: duration,
		FPS:         30,
		Resolution: map[string]interface{}{
			"width":  float64(1920),
			"height": float64(1080),
		},
		Tracks: []map[string]interface{}{
			{
				"id":    "shotlist-overlay",
				"type":  "overlay",
				"items": overlayItems,
			},
			{
				"id":    "shotlist-caption",
				"type":  "caption",
				"items": captionItems,
			},
		},
		Style: map[string]interface{}{
			"theme":      "open_source_launch",
			"background": "production_dark",
		},
	}
}

func shotListItemsFromPayload(payload map[string]interface{}) []interface{} {
	if payload == nil {
		return nil
	}
	if items := interfaceSlice(payload["shotList"]); len(items) > 0 {
		return items
	}
	if items := interfaceSlice(payload["shots"]); len(items) > 0 {
		return items
	}
	for _, key := range []string{"shotList", "shots"} {
		if wrapper := mapFromMap(payload, key); wrapper != nil {
			if items := interfaceSlice(wrapper["shotList"]); len(items) > 0 {
				return items
			}
			if items := interfaceSlice(wrapper["shots"]); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

// extractCardsAndCaptions extracts card and caption info from the tracks.
func extractCardsAndCaptions(spec *compositionSpec) ([]cardInfo, []captionInfo) {
	var cards []cardInfo
	var captions []captionInfo

	for _, track := range spec.Tracks {
		trackType, _ := track["type"].(string)
		items, _ := track["items"].([]interface{})

		switch trackType {
		case "overlay":
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				c := cardInfo{
					ID:        stringFromMap(m, "id"),
					Kind:      stringFromMap(m, "kind"),
					Title:     stringFromMap(m, "title"),
					Body:      stringFromMap(m, "body"),
					Animation: stringFromMap(m, "animation"),
				}
				if start, ok := m["startSec"].(float64); ok {
					c.StartSec = start
				}
				if end, ok := m["endSec"].(float64); ok {
					c.EndSec = end
				}
				cards = append(cards, c)
			}
		case "caption":
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				c := captionInfo{
					ID:   stringFromMap(m, "id"),
					Text: stringFromMap(m, "text"),
				}
				if start, ok := m["startSec"].(float64); ok {
					c.StartSec = start
				}
				if end, ok := m["endSec"].(float64); ok {
					c.EndSec = end
				}
				captions = append(captions, c)
			}
		}
	}
	return cards, captions
}

func stringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func floatFromMap(m map[string]interface{}, key string, fallback float64) float64 {
	if value, ok := floatValueFromMap(m, key); ok {
		return value
	}
	return fallback
}

func floatValueFromMap(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}

func mapFromInterface(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return v
	case map[string]string:
		m := make(map[string]interface{}, len(v))
		for key, item := range v {
			m[key] = item
		}
		return m
	case string:
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(v), &m); err == nil {
			return m
		}
	}
	return nil
}

func mapFromMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	return mapFromInterface(m[key])
}

func interfaceSlice(value interface{}) []interface{} {
	switch v := value.(type) {
	case []interface{}:
		return v
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return items
	case string:
		var items []interface{}
		if err := json.Unmarshal([]byte(v), &items); err == nil {
			return items
		}
	}
	return nil
}

func firstStringFromMap(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFromMap(m, key)); value != "" {
			return value
		}
	}
	return ""
}

func shotMediaPackagesFromPayload(dataDir, projectID, projectRoot string, payload map[string]interface{}) []shotMediaPackage {
	if payload == nil {
		return nil
	}
	items := interfaceSlice(payload["shotAssetPackages"])
	if len(items) == 0 {
		return nil
	}

	packages := make([]shotMediaPackage, 0, len(items))
	nextStart := 0.0
	for index, item := range items {
		m := mapFromInterface(item)
		if m == nil {
			continue
		}

		generationPlan := mapFromMap(m, "generationPlan")
		fusionPlan := mapFromMap(generationPlan, "fusionPlan")
		baseLayerMap := mapFromMap(fusionPlan, "baseLayer")
		overlayItems := interfaceSlice(fusionPlan["overlayLayers"])
		screenText := firstStringFromMap(m, "screenText")
		if baseLayerMap == nil && len(overlayItems) == 0 && screenText == "" {
			continue
		}

		shotID := strings.TrimSpace(stringFromMap(m, "shotId"))
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", index+1)
		}
		duration := floatFromMap(m, "durationSec", 4)
		if duration <= 0 {
			duration = 4
		}

		baseLayer := mediaLayer{
			Kind:       strings.TrimSpace(stringFromMap(baseLayerMap, "kind")),
			StorageRef: strings.TrimSpace(stringFromMap(baseLayerMap, "storageRef")),
		}
		baseLayer.ResolvedSrc, baseLayer.Missing = resolveMediaStorageRef(dataDir, projectID, projectRoot, baseLayer.StorageRef)

		overlays := make([]mediaOverlay, 0, len(overlayItems))
		for overlayIndex, overlayItem := range overlayItems {
			overlayMap := mapFromInterface(overlayItem)
			if overlayMap == nil {
				continue
			}
			text := firstStringFromMap(overlayMap, "text", "label", "title")
			if text == "" {
				continue
			}
			overlayID := strings.TrimSpace(stringFromMap(overlayMap, "id"))
			if overlayID == "" {
				overlayID = fmt.Sprintf("overlay-%d", overlayIndex+1)
			}
			overlays = append(overlays, mediaOverlay{
				ID:   overlayID,
				Kind: strings.TrimSpace(stringFromMap(overlayMap, "kind")),
				Role: strings.TrimSpace(stringFromMap(overlayMap, "role")),
				Text: text,
			})
		}
		if len(overlays) == 0 && screenText != "" {
			overlays = append(overlays, mediaOverlay{ID: "screen-text", Kind: "html_overlay", Role: "screen_text", Text: screenText})
		}

		packages = append(packages, shotMediaPackage{
			ShotID:      shotID,
			DurationSec: duration,
			Mode:        strings.TrimSpace(stringFromMap(generationPlan, "mode")),
			BaseLayer:   baseLayer,
			Overlays:    overlays,
			StartSec:    nextStart,
		})
		nextStart += duration
	}
	return packages
}

func resolveMediaStorageRef(dataDir, projectID, projectRoot, storageRef string) (string, bool) {
	storageRef = strings.TrimSpace(storageRef)
	if storageRef == "" {
		return "", true
	}
	if strings.HasPrefix(storageRef, "http://") || strings.HasPrefix(storageRef, "https://") {
		return storageRef, false
	}

	const prefix = "local://projects/"
	if !strings.HasPrefix(storageRef, prefix) {
		return "", true
	}
	localPath := strings.TrimPrefix(storageRef, prefix)
	if strings.ContainsAny(localPath, "?#") {
		return "", true
	}
	parts := strings.Split(localPath, "/")
	if len(parts) != 5 || parts[1] != "artifacts" {
		return "", true
	}
	refProjectID := parts[0]
	artifactID := parts[2]
	hash := parts[3]
	name := parts[4]
	if refProjectID != projectID {
		return "", true
	}
	if err := validateLocalSegment(refProjectID); err != nil {
		return "", true
	}
	if err := validateLocalSegment(artifactID); err != nil {
		return "", true
	}
	if err := validateLocalSegment(hash); err != nil {
		return "", true
	}
	if err := validateLocalSegment(name); err != nil {
		return "", true
	}

	contentPath := filepath.Join(dataDir, "artifacts", refProjectID, artifactID, "content")
	if err := ensureInside(dataDir, contentPath); err != nil {
		return "", true
	}
	if info, err := os.Stat(contentPath); err != nil || info.IsDir() {
		return "", true
	}
	if strings.TrimSpace(projectRoot) != "" {
		if localSrc, err := copyLocalMediaIntoProject(projectRoot, contentPath, artifactID, hash, name); err == nil {
			return localSrc, false
		}
	}
	return localAgentRawArtifactURL(refProjectID, artifactID), false
}

func copyLocalMediaIntoProject(projectRoot, contentPath, artifactID, hash, name string) (string, error) {
	shortHash := hash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}
	fileName := artifactID + "-" + shortHash + "-" + name
	if err := validateLocalSegment(fileName); err != nil {
		return "", err
	}
	mediaDir := filepath.Join(projectRoot, "assets", "media")
	if err := ensureInside(projectRoot, mediaDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return "", err
	}
	destPath := filepath.Join(mediaDir, fileName)
	if err := ensureInside(projectRoot, destPath); err != nil {
		return "", err
	}
	source, err := os.Open(contentPath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	target, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return "", err
	}
	if err := target.Close(); err != nil {
		return "", err
	}
	return path.Join("assets", "media", fileName), nil
}

func localAgentRawArtifactURL(projectID, artifactID string) string {
	baseURL := strings.TrimSpace(os.Getenv("TANGYING_LOCAL_AGENT_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:18080"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + "/api/local/artifacts/" + url.PathEscape(artifactID) + "?projectId=" + url.QueryEscape(projectID) + "&raw=1"
}

// ---- HTML/CSS generation ----

func buildCompositionIndex(topic string, spec *compositionSpec, cards []cardInfo, captions []captionInfo, style map[string]interface{}) string {
	if topic == "" {
		topic = "Tangying AIOS Video"
	}
	duration := spec.DurationSec
	if duration <= 0 {
		duration = 45
	}
	fps := int(spec.FPS)
	if fps <= 0 {
		fps = 30
	}
	if len(cards) == 0 {
		cards = []cardInfo{{
			ID:       "card-default",
			Kind:     "title_card",
			StartSec: 0,
			EndSec:   duration,
			Title:    topic,
			Body:     "AIOS 已生成可审核的视频预览项目。",
		}}
	}
	cards = normalizedCards(cards, duration)
	captions = normalizedCaptions(captions, duration)

	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + html.EscapeString(topic) + `</title>
  <link rel="stylesheet" href="assets/style.css" />
</head>
<body>
  <div data-composition-id="tangying-main" data-start="0" data-duration="` + fmt.Sprintf("%.1f", duration) + `" data-track-index="0" data-width="1920" data-height="1080">
    <div class="scene-content">
      <div class="bg-layer" data-layout-ignore>
        <div class="bg-field"></div>
        <div class="bg-orbit orbit-one"></div>
        <div class="bg-orbit orbit-two"></div>
        <div class="bg-word">AIOS</div>
      </div>
      <div class="project-kicker">Tangying Director Studio</div>
      <div class="card-layer">
`)

	// Generate card elements.
	cardIDs := make([]string, 0, len(cards))
	for index, card := range cards {
		cardID := safeElementID(card.ID, fmt.Sprintf("card-%d", index+1))
		cardIDs = append(cardIDs, cardID)
		kind := safeElementID(card.Kind, "card")
		b.WriteString(fmt.Sprintf(`        <article id="%s" class="video-card card-%s" data-kind="%s">
          <div class="card-meta">%02d / %02d</div>
          <h1 class="card-title">%s</h1>
          <p class="card-body">%s</p>
        </article>
`, cardID, kind, html.EscapeString(card.Kind), index+1, len(cards), html.EscapeString(card.Title), html.EscapeString(card.Body)))
	}

	b.WriteString(`      </div>
      <div class="caption-layer">
`)

	captionIDs := make([]string, 0, len(captions))
	for index, cap := range captions {
		captionID := safeElementID(cap.ID, fmt.Sprintf("caption-%d", index+1))
		captionIDs = append(captionIDs, captionID)
		b.WriteString(fmt.Sprintf(`        <p id="%s" class="caption">%s</p>
`, captionID, html.EscapeString(cap.Text)))
	}

	b.WriteString(`      </div>
      <div class="progress-bar" data-layout-ignore>
        <div class="progress-fill"></div>
      </div>
    </div>
  </div>
  <script src="https://cdn.jsdelivr.net/npm/gsap@3.14.2/dist/gsap.min.js"></script>
  <script>
    window.__timelines = window.__timelines || {};
    const tl = gsap.timeline({ paused: true, defaults: { duration: 0.55, ease: "power2.out" } });
    tl.set(".video-card, .caption", { opacity: 0, y: 0, scale: 1 }, 0);
    tl.set(".progress-fill", { opacity: 1, scaleX: 0, transformOrigin: "left center" }, 0);
    tl.to(".progress-fill", { scaleX: 1, duration: ` + fmt.Sprintf("%.3f", duration) + `, ease: "none" }, 0);
    tl.from(".project-kicker", { opacity: 0, y: -24, duration: 0.5, ease: "power3.out" }, 0.2);
    tl.to(".bg-orbit", { rotation: 18, scale: 1.04, duration: ` + fmt.Sprintf("%.3f", duration) + `, ease: "sine.inOut", stagger: 0.2 }, 0);
`)

	for index, card := range cards {
		cardID := cardIDs[index]
		start := clampTime(card.StartSec, duration)
		end := clampTime(card.EndSec, duration)
		if end <= start {
			end = duration
		}
		enterAt := start + 0.15
		if enterAt > duration {
			enterAt = start
		}
		exitAt := end - 0.35
		if exitAt < enterAt+0.7 {
			exitAt = enterAt + 0.7
		}
		if exitAt > duration-0.35 {
			exitAt = duration - 0.35
		}
		if exitAt < enterAt {
			exitAt = enterAt
		}
		b.WriteString(fmt.Sprintf(`    tl.fromTo("#%s", { opacity: 0, y: 64, scale: 0.96 }, { opacity: 1, y: 0, scale: 1, duration: 0.58, ease: "power3.out" }, %.3f);
    tl.to("#%s", { opacity: 0, y: -28, duration: 0.35, ease: "power2.in" }, %.3f);
`, cardID, enterAt, cardID, exitAt))
	}
	for index, cap := range captions {
		captionID := captionIDs[index]
		start := clampTime(cap.StartSec, duration)
		end := clampTime(cap.EndSec, duration)
		if end <= start {
			end = duration
		}
		exitAt := end - 0.25
		if exitAt < start {
			exitAt = start
		}
		b.WriteString(fmt.Sprintf(`    tl.fromTo("#%s", { opacity: 0, y: 18 }, { opacity: 1, y: 0, duration: 0.28, ease: "sine.out" }, %.3f);
    tl.to("#%s", { opacity: 0, y: -12, duration: 0.24, ease: "sine.in" }, %.3f);
`, captionID, start, captionID, exitAt))
	}

	b.WriteString(`    window.__timelines["tangying-main"] = tl;
  </script>
</body>
</html>
`)

	return b.String()
}

func buildCompositionStyle() string {
	return `/* OneClick Video — HyperFrames composition styles */

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  width: 1920px;
  height: 1080px;
  overflow: hidden;
  font-family: "Aptos Display", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  background: #101820;
  color: #f2efe4;
}

[data-composition-id="tangying-main"] {
  width: 1920px;
  height: 1080px;
  overflow: hidden;
  position: relative;
  background: #101820;
}

.scene-content {
  position: relative;
  width: 100%;
  height: 100%;
  padding: 92px 132px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  box-sizing: border-box;
}

.bg-layer {
  position: absolute;
  inset: 0;
  z-index: 0;
}
.shot-media,
.missing-media {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  z-index: 1;
  opacity: 0;
  pointer-events: none;
  will-change: opacity;
}
.missing-media {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 80px;
  background:
    linear-gradient(135deg, rgba(16, 24, 32, 0.9), rgba(79, 138, 139, 0.55)),
    repeating-linear-gradient(45deg, rgba(242, 239, 228, 0.08) 0 12px, transparent 12px 24px);
  color: rgba(242, 239, 228, 0.9);
  font-size: 46px;
  font-weight: 850;
  line-height: 1.2;
  text-align: center;
}
.media-safety-mask {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 38%;
  z-index: 5;
  pointer-events: none;
  background: linear-gradient(
    to top,
    rgba(16, 24, 32, 0.94) 0%,
    rgba(16, 24, 32, 0.78) 46%,
    rgba(16, 24, 32, 0) 100%
  );
}
.bg-field {
  position: absolute;
  inset: 0;
  z-index: 0;
  width: 100%;
  height: 100%;
  background:
    radial-gradient(circle at 14% 18%, rgba(200, 107, 52, 0.32), transparent 28%),
    radial-gradient(circle at 88% 76%, rgba(79, 138, 139, 0.36), transparent 32%),
    linear-gradient(135deg, #101820 0%, #18222d 58%, #241812 100%);
}
.bg-orbit {
  position: absolute;
  border: 1px solid rgba(242, 239, 228, 0.12);
  border-radius: 50%;
  transform-origin: center;
}
.orbit-one {
  width: 760px;
  height: 760px;
  right: -160px;
  top: -210px;
}
.orbit-two {
  width: 560px;
  height: 560px;
  left: -120px;
  bottom: -180px;
}
.bg-word {
  position: absolute;
  right: 78px;
  bottom: 42px;
  font-size: 150px;
  font-weight: 900;
  letter-spacing: 0;
  color: rgba(242, 239, 228, 0.045);
}

.project-kicker {
  position: relative;
  z-index: 2;
  align-self: flex-start;
  margin-bottom: 30px;
  padding: 12px 18px;
  border: 1px solid rgba(242, 239, 228, 0.22);
  color: #f0c28b;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.card-layer {
  position: relative;
  z-index: 10;
  min-height: 520px;
}

.video-card {
  position: absolute;
  inset: 0 auto auto 0;
  width: min(1320px, 82%);
  min-height: 450px;
  padding: 62px 74px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  border: 1px solid rgba(242, 239, 228, 0.18);
  background: rgba(16, 24, 32, 0.74);
  box-shadow: 0 26px 80px rgba(0, 0, 0, 0.26);
  backdrop-filter: blur(18px);
  will-change: transform, opacity;
}

.video-card::before {
  content: "";
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 10px;
  background: #c86b34;
}

.card-summary_card::before {
  background: #4f8a8b;
}

.card-knowledge_card::before {
  background: #d6a44f;
}

.card-meta {
  color: #f0c28b;
  font-family: "IBM Plex Mono", ui-monospace, monospace;
  font-size: 24px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.card-title {
  margin-top: 22px;
  max-width: 1120px;
  color: #f2efe4;
  font-size: 82px;
  font-weight: 900;
  line-height: 1.08;
  letter-spacing: 0;
  overflow-wrap: anywhere;
}

.card-body {
  margin-top: 28px;
  max-width: 1120px;
  color: rgba(242, 239, 228, 0.82);
  font-size: 34px;
  font-weight: 350;
  line-height: 1.48;
  letter-spacing: 0;
  overflow-wrap: anywhere;
}

.caption-layer {
  position: relative;
  z-index: 20;
  min-height: 96px;
  margin-top: 26px;
}

.caption {
  position: absolute;
  left: 0;
  bottom: 0;
  max-width: 1180px;
  padding: 16px 24px;
  border: 1px solid rgba(242, 239, 228, 0.14);
  background: rgba(242, 239, 228, 0.1);
  color: rgba(242, 239, 228, 0.9);
  font-size: 28px;
  font-weight: 500;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.progress-bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 8px;
  background: rgba(242, 239, 228, 0.12);
  z-index: 30;
}

.progress-fill {
  width: 100%;
  height: 100%;
  background: #c86b34;
  transform-origin: left center;
}
`
}

func normalizedCards(cards []cardInfo, duration float64) []cardInfo {
	result := make([]cardInfo, 0, len(cards))
	for index, card := range cards {
		if strings.TrimSpace(card.ID) == "" {
			card.ID = fmt.Sprintf("card-%d", index+1)
		}
		if strings.TrimSpace(card.Kind) == "" {
			card.Kind = "content_card"
		}
		if strings.TrimSpace(card.Title) == "" {
			card.Title = fmt.Sprintf("画面 %d", index+1)
		}
		if strings.TrimSpace(card.Body) == "" {
			card.Body = "这一段展示当前视频的核心信息。"
		}
		card.StartSec = clampTime(card.StartSec, duration)
		card.EndSec = clampTime(card.EndSec, duration)
		if card.EndSec <= card.StartSec {
			card.EndSec = duration
		}
		result = append(result, card)
	}
	return result
}

func normalizedCaptions(captions []captionInfo, duration float64) []captionInfo {
	result := make([]captionInfo, 0, len(captions))
	for index, caption := range captions {
		if strings.TrimSpace(caption.ID) == "" {
			caption.ID = fmt.Sprintf("caption-%d", index+1)
		}
		if strings.TrimSpace(caption.Text) == "" {
			continue
		}
		caption.StartSec = clampTime(caption.StartSec, duration)
		caption.EndSec = clampTime(caption.EndSec, duration)
		if caption.EndSec <= caption.StartSec {
			caption.EndSec = duration
		}
		result = append(result, caption)
	}
	return result
}

func clampTime(value, duration float64) float64 {
	if value < 0 {
		return 0
	}
	if duration > 0 && value > duration {
		return duration
	}
	return value
}

func safeElementID(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	id := strings.Trim(b.String(), "-_")
	if id == "" {
		id = fallback
	}
	first := id[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		id = "el-" + id
	}
	return id
}

func fallbackCompositionSpec() *compositionSpec {
	return &compositionSpec{
		SpecVersion: "oneclick-video/v1",
		ProjectType: "image_text_video",
		DurationSec: 8,
		FPS:         30,
		Resolution: map[string]interface{}{
			"width":  float64(1920),
			"height": float64(1080),
		},
		Style: map[string]interface{}{
			"theme":      "editorial",
			"background": "warm-dark",
		},
	}
}

func buildFallbackCards(topic, script string) []cardInfo {
	if strings.TrimSpace(script) == "" {
		script = "AIOS 已完成项目骨架生成，下一步进入预览与渲染。"
	}
	return []cardInfo{
		{
			ID:       "card-intro",
			Kind:     "title_card",
			StartSec: 0,
			EndSec:   4,
			Title:    topic,
			Body:     script,
		},
		{
			ID:       "card-output",
			Kind:     "summary_card",
			StartSec: 4,
			EndSec:   8,
			Title:    "可审核，可播放",
			Body:     "中间产物沉淀为项目文件，最终视频写入本地 artifact，客户端可以直接预览。",
		},
	}
}

func buildFallbackCaptions() []captionInfo {
	return []captionInfo{
		{ID: "caption-intro", StartSec: 0, EndSec: 4, Text: "一句话进入 AI 导演台"},
		{ID: "caption-output", StartSec: 4, EndSec: 8, Text: "最终视频将在客户端播放器中出现"},
	}
}

func buildHyperFramesIndex(topic, script string) string {
	if topic == "" {
		topic = "Tangying AIOS Video"
	}
	spec := fallbackCompositionSpec()
	return buildCompositionIndex(topic, spec, buildFallbackCards(topic, script), buildFallbackCaptions(), spec.Style)
}

func buildHyperFramesIndexWithMedia(topic, script string, mediaPackages []shotMediaPackage) string {
	return buildHyperFramesIndexWithMediaSpec(topic, script, mediaPackages, nil)
}

func buildHyperFramesIndexWithMediaSpec(topic, script string, mediaPackages []shotMediaPackage, spec *compositionSpec) string {
	if len(mediaPackages) == 0 {
		return buildHyperFramesIndex(topic, script)
	}
	if topic == "" {
		topic = "Tangying AIOS Video"
	}
	if spec == nil {
		spec = fallbackCompositionSpec()
		if duration := totalMediaDuration(mediaPackages); duration > 0 {
			spec.DurationSec = duration
		}
	}
	if spec.DurationSec <= 0 {
		if duration := totalMediaDuration(mediaPackages); duration > 0 {
			spec.DurationSec = duration
		} else {
			spec.DurationSec = 8
		}
	}
	cards, captions := mediaPackageCards(topic, script, mediaPackages), mediaPackageCaptions(mediaPackages)
	if len(spec.Tracks) > 0 {
		if shotCards, shotCaptions := extractCardsAndCaptions(spec); len(shotCards) > 0 {
			cards = shotCards
			captions = shotCaptions
		}
	}
	index := buildCompositionIndex(topic, spec, cards, captions, spec.Style)
	return injectTimedMedia(index, mediaPackages)
}

func totalMediaDuration(mediaPackages []shotMediaPackage) float64 {
	total := 0.0
	for _, pkg := range mediaPackages {
		duration := pkg.DurationSec
		if duration <= 0 {
			duration = 4
		}
		if end := pkg.StartSec + duration; end > total {
			total = end
		}
	}
	return total
}

type shotTiming struct {
	StartSec    float64
	DurationSec float64
}

func alignMediaPackagesToComposition(mediaPackages []shotMediaPackage, spec *compositionSpec) []shotMediaPackage {
	if len(mediaPackages) == 0 || spec == nil {
		return mediaPackages
	}
	timings := shotTimingsFromComposition(spec)
	if len(timings) == 0 {
		return mediaPackages
	}
	aligned := make([]shotMediaPackage, len(mediaPackages))
	copy(aligned, mediaPackages)
	for index := range aligned {
		shotID := strings.TrimSpace(aligned[index].ShotID)
		if timing, ok := timings[shotID]; ok {
			aligned[index].StartSec = timing.StartSec
			if timing.DurationSec > 0 {
				aligned[index].DurationSec = timing.DurationSec
			}
		}
	}
	return aligned
}

func shotTimingsFromComposition(spec *compositionSpec) map[string]shotTiming {
	timings := map[string]shotTiming{}
	if spec == nil {
		return timings
	}
	for _, track := range spec.Tracks {
		if stringFromMap(track, "type") != "overlay" {
			continue
		}
		for _, item := range interfaceSlice(track["items"]) {
			m := mapFromInterface(item)
			if m == nil {
				continue
			}
			shotID := firstStringFromMap(m, "shotId")
			if shotID == "" {
				shotID = strings.TrimPrefix(stringFromMap(m, "id"), "card-")
			}
			shotID = strings.TrimSpace(shotID)
			if shotID == "" {
				continue
			}
			start := floatFromMap(m, "startSec", 0)
			end := floatFromMap(m, "endSec", start)
			if end <= start {
				continue
			}
			timings[shotID] = shotTiming{StartSec: start, DurationSec: end - start}
		}
	}
	return timings
}

func mediaPackageCards(topic, script string, mediaPackages []shotMediaPackage) []cardInfo {
	cards := make([]cardInfo, 0, len(mediaPackages))
	for index, pkg := range mediaPackages {
		duration := pkg.DurationSec
		if duration <= 0 {
			duration = 4
		}
		title := pkg.ShotID
		if index == 0 && strings.TrimSpace(topic) != "" {
			title = topic
		}
		cards = append(cards, cardInfo{
			ID:       "card-" + pkg.ShotID,
			Kind:     "content_card",
			StartSec: pkg.StartSec,
			EndSec:   pkg.StartSec + duration,
			Title:    title,
			Body:     mediaPackageBody(pkg, script),
		})
	}
	return cards
}

func mediaPackageBody(pkg shotMediaPackage, script string) string {
	for _, overlay := range pkg.Overlays {
		if strings.TrimSpace(overlay.Text) != "" {
			return overlay.Text
		}
	}
	if strings.TrimSpace(script) != "" {
		return script
	}
	if pkg.BaseLayer.Missing {
		return "素材暂缺，渲染时显示占位层。"
	}
	return "素材已接入本地 HyperFrames 预览。"
}

func mediaPackageCaptions(mediaPackages []shotMediaPackage) []captionInfo {
	var captions []captionInfo
	for pkgIndex, pkg := range mediaPackages {
		duration := pkg.DurationSec
		if duration <= 0 {
			duration = 4
		}
		for overlayIndex, overlay := range pkg.Overlays {
			if strings.TrimSpace(overlay.Text) == "" {
				continue
			}
			id := overlay.ID
			if strings.TrimSpace(id) == "" {
				id = fmt.Sprintf("overlay-%d", overlayIndex+1)
			}
			captions = append(captions, captionInfo{
				ID:       fmt.Sprintf("caption-%d-%s", pkgIndex+1, id),
				StartSec: pkg.StartSec,
				EndSec:   pkg.StartSec + duration,
				Text:     overlay.Text,
			})
		}
	}
	return captions
}

func injectTimedMedia(index string, mediaPackages []shotMediaPackage) string {
	bgMarker := "      <div class=\"bg-layer\" data-layout-ignore>\n"
	index = strings.Replace(index, bgMarker, bgMarker+timedMediaElements(mediaPackages)+`        <div class="media-safety-mask" data-layout-ignore></div>
`, 1)

	setMarker := "    tl.set(\".video-card, .caption\", { opacity: 0, y: 0, scale: 1 }, 0);\n"
	index = strings.Replace(index, setMarker, setMarker+"    tl.set(\".shot-media, .missing-media\", { opacity: 0 }, 0);\n", 1)

	timelineMarker := "    window.__timelines[\"tangying-main\"] = tl;"
	index = strings.Replace(index, timelineMarker, timedMediaTimeline(mediaPackages)+timelineMarker, 1)
	return index
}

func timedMediaElements(mediaPackages []shotMediaPackage) string {
	var b strings.Builder
	for index, pkg := range mediaPackages {
		duration := pkg.DurationSec
		if duration <= 0 {
			duration = 4
		}
		shotID := html.EscapeString(pkg.ShotID)
		elementID := safeMediaElementID(pkg, index)
		if pkg.BaseLayer.Missing || pkg.BaseLayer.ResolvedSrc == "" {
			b.WriteString(fmt.Sprintf(`        <div id="%s" class="missing-media" data-shot-id="%s" data-start="%.1f" data-duration="%.1f" data-track-index="%d">Missing media for %s</div>
`, elementID, shotID, pkg.StartSec, duration, index, shotID))
			continue
		}
		src := html.EscapeString(pkg.BaseLayer.ResolvedSrc)
		switch strings.ToLower(pkg.BaseLayer.Kind) {
		case "image", "img", "still":
			b.WriteString(fmt.Sprintf(`        <img id="%s" class="shot-media" data-shot-id="%s" data-start="%.1f" data-duration="%.1f" data-track-index="%d" src="%s" crossorigin="anonymous" alt="" />
`, elementID, shotID, pkg.StartSec, duration, index, src))
		default:
			b.WriteString(fmt.Sprintf(`        <video id="%s" class="shot-media" data-shot-id="%s" data-start="%.1f" data-duration="%.1f" data-track-index="%d" src="%s" muted playsinline crossorigin="anonymous"></video>
`, elementID, shotID, pkg.StartSec, duration, index, src))
		}
	}
	return b.String()
}

func timedMediaTimeline(mediaPackages []shotMediaPackage) string {
	var b strings.Builder
	for index, pkg := range mediaPackages {
		duration := pkg.DurationSec
		if duration <= 0 {
			duration = 4
		}
		start := pkg.StartSec
		exitAt := start + duration - 0.2
		if exitAt < start {
			exitAt = start
		}
		elementID := safeMediaElementID(pkg, index)
		b.WriteString(fmt.Sprintf(`    tl.fromTo("#%s", { opacity: 0 }, { opacity: 1, duration: 0.18, ease: "none" }, %.3f);
    tl.to("#%s", { opacity: 0, duration: 0.18, ease: "none" }, %.3f);
`, elementID, start, elementID, exitAt))
	}
	return b.String()
}

func safeMediaElementID(pkg shotMediaPackage, index int) string {
	prefix := "media-"
	if pkg.BaseLayer.Missing || pkg.BaseLayer.ResolvedSrc == "" {
		prefix = "missing-"
	}
	return safeElementID(prefix+pkg.ShotID, fmt.Sprintf("%s%d", prefix, index+1))
}

func buildDesignDoc(projectID, topic string, spec *compositionSpec) string {
	var b strings.Builder
	b.WriteString("# HyperFrames 项目设计文档\n\n")
	b.WriteString(fmt.Sprintf("- **项目 ID**: %s\n", projectID))
	b.WriteString(fmt.Sprintf("- **主题**: %s\n", topic))
	b.WriteString(fmt.Sprintf("- **版本**: %s\n", spec.SpecVersion))
	b.WriteString(fmt.Sprintf("- **类型**: %s\n", spec.ProjectType))
	b.WriteString(fmt.Sprintf("- **时长**: %.1fs\n", spec.DurationSec))
	b.WriteString(fmt.Sprintf("- **帧率**: %.0f fps\n", spec.FPS))
	if res, ok := spec.Resolution["width"].(float64); ok {
		if h, ok2 := spec.Resolution["height"].(float64); ok2 {
			b.WriteString(fmt.Sprintf("- **分辨率**: %.0f×%.0f\n", res, h))
		}
	}
	b.WriteString("\n## 轨道结构\n\n")
	for _, track := range spec.Tracks {
		trackType, _ := track["type"].(string)
		trackID, _ := track["id"].(string)
		items, _ := track["items"].([]interface{})
		b.WriteString(fmt.Sprintf("### %s (%s)\n\n", trackID, trackType))
		b.WriteString(fmt.Sprintf("- 元素数: %d\n", len(items)))
		b.WriteString("\n")
	}
	b.WriteString("\n## 样式\n\n")
	b.WriteString(fmt.Sprintf("- 主题: %v\n", spec.Style["theme"]))
	b.WriteString(fmt.Sprintf("- 背景: %v\n", spec.Style["background"]))
	b.WriteString("\n## 渲染说明\n\n")
	b.WriteString("1. 使用 HyperFrames Render Service 渲染\n")
	b.WriteString("2. 卡片按时间轴自动显示/隐藏\n")
	b.WriteString("3. 字幕同步叠加在底部\n")
	b.WriteString("4. 进度条指示渲染进度\n")
	return b.String()
}

func stringFromPayload(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}
