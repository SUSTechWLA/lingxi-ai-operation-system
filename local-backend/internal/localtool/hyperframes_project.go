package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
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
			"topic":        topic,
			"script":       script,
			"shotList":     job.Payload["shotList"],
			"videoPrompts": job.Payload["videoPrompts"],
			"style":        job.Payload["style"],
			"publishCopy":  job.Payload["publishCopy"],
		}
		dataJSON, _ := json.MarshalIndent(data, "", "  ")
		if err := os.WriteFile(filepath.Join(assetsDir, "data.json"), dataJSON, 0o644); err != nil {
			return nil, err
		}

		index := buildHyperFramesIndex(topic, script)
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

		files = []string{localRef + "/index.html", localRef + "/assets/data.json", localRef + "/manifest.json"}
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
	totalFrames := int(duration) * fps

	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + html.EscapeString(topic) + `</title>
  <link rel="stylesheet" href="assets/style.css" />
  <style>
    :root {
      --duration: ` + fmt.Sprintf("%.1f", duration) + `s;
      --fps: ` + fmt.Sprintf("%d", fps) + `;
      --total-frames: ` + fmt.Sprintf("%d", totalFrames) + `;
    }
  </style>
</head>
<body>
  <!-- Background layer -->
  <div class="bg-layer">
    <div class="bg-gradient"></div>
  </div>

  <!-- Card (overlay) layer -->
  <div class="card-layer">
`)

	// Generate card elements.
	for _, card := range cards {
		startPct := card.StartSec / duration * 100
		endPct := card.EndSec / duration * 100
		anim := card.Animation
		if anim == "" {
			anim = "fade_in"
		}
		b.WriteString(fmt.Sprintf(`    <div class="card card-%s" data-kind="%s" style="--start: %.2f%%; --end: %.2f%%; --anim: %s">
      <div class="card-inner">
        <h2 class="card-title">%s</h2>
        <p class="card-body">%s</p>
      </div>
    </div>
`, card.Kind, card.Kind, startPct, endPct, anim, html.EscapeString(card.Title), html.EscapeString(card.Body)))
	}

	b.WriteString(`  </div>

  <!-- Caption layer -->
  <div class="caption-layer">
`)

	for _, cap := range captions {
		startPct := cap.StartSec / duration * 100
		endPct := cap.EndSec / duration * 100
		b.WriteString(fmt.Sprintf(`    <div class="caption" style="--start: %.2f%%; --end: %.2f%%">%s</div>
`, startPct, endPct, html.EscapeString(cap.Text)))
	}

	b.WriteString(`  </div>

  <!-- Progress bar -->
  <div class="progress-bar">
    <div class="progress-fill"></div>
  </div>

  <script>
    // Card and caption timeline animation.
    const duration = ` + fmt.Sprintf("%.1f", duration) + `;
    const cards = document.querySelectorAll('.card');
    const captions = document.querySelectorAll('.caption');
    const progressFill = document.querySelector('.progress-fill');

    function updateTimeline(currentSec) {
      const pct = (currentSec / duration) * 100;
      progressFill.style.width = pct + '%';

      cards.forEach(card => {
        const start = parseFloat(card.style.getPropertyValue('--start'));
        const end = parseFloat(card.style.getPropertyValue('--end'));
        if (pct >= start && pct <= end) {
          card.classList.add('visible');
          card.classList.remove('exit');
        } else if (pct > end) {
          card.classList.add('exit');
          card.classList.remove('visible');
        } else {
          card.classList.remove('visible', 'exit');
        }
      });

      captions.forEach(cap => {
        const start = parseFloat(cap.style.getPropertyValue('--start'));
        const end = parseFloat(cap.style.getPropertyValue('--end'));
        cap.style.opacity = (pct >= start && pct <= end) ? '1' : '0';
      });
    }

    // For rendered video: update on each animation frame (60fps assumed).
    let currentFrame = 0;
    const totalFrames = ` + fmt.Sprintf("%d", totalFrames) + `;
    function tick() {
      const currentSec = (currentFrame / totalFrames) * duration;
      updateTimeline(currentSec);
      currentFrame++;
      if (currentFrame <= totalFrames) {
        requestAnimationFrame(tick);
      }
    }
    tick();
  </script>
</body>
</html>
`)

	return b.String()
}

func buildCompositionStyle() string {
	return `/* OneClick Video — Image Text Video Styles */

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  width: 1920px;
  height: 1080px;
  overflow: hidden;
  font-family: "PingFang SC", "Noto Sans SC", "Microsoft YaHei", system-ui, sans-serif;
  background: #0f0f1a;
  color: #ffffff;
  position: relative;
}

/* Background layer */
.bg-layer {
  position: absolute;
  inset: 0;
  z-index: 0;
}
.bg-gradient {
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
}

/* Card layer */
.card-layer {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
}

.card {
  position: absolute;
  width: 80%;
  max-width: 1400px;
  opacity: 0;
  transform: translateY(40px);
  transition: opacity 0.5s ease, transform 0.5s ease;
}
.card.visible {
  opacity: 1;
  transform: translateY(0);
}
.card.exit {
  opacity: 0;
  transform: translateY(-30px);
}

/* Card kinds */
.card-title_card .card-inner {
  border-left: 6px solid #e94560;
  background: rgba(233, 69, 96, 0.08);
}
.card-knowledge_card .card-inner {
  border-left: 6px solid #0f3460;
  background: rgba(15, 52, 96, 0.15);
}
.card-summary_card .card-inner {
  border-left: 6px solid #16c79a;
  background: rgba(22, 199, 154, 0.08);
}

.card-inner {
  padding: 60px 80px;
  border-radius: 16px;
  backdrop-filter: blur(10px);
}

.card-title {
  font-size: 56px;
  font-weight: 700;
  margin-bottom: 24px;
  line-height: 1.3;
  letter-spacing: -0.02em;
}

.card-body {
  font-size: 32px;
  line-height: 1.6;
  opacity: 0.85;
  max-width: 1200px;
}

/* Caption layer */
.caption-layer {
  position: absolute;
  bottom: 80px;
  left: 0;
  right: 0;
  z-index: 20;
  display: flex;
  justify-content: center;
  padding: 0 120px;
}

.caption {
  position: absolute;
  font-size: 28px;
  font-weight: 500;
  color: #ffffff;
  text-align: center;
  background: rgba(0, 0, 0, 0.65);
  padding: 12px 32px;
  border-radius: 8px;
  max-width: 85%;
  line-height: 1.5;
  transition: opacity 0.3s ease;
}

/* Progress bar */
.progress-bar {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
  z-index: 30;
}
.progress-fill {
  height: 100%;
  width: 0%;
  background: linear-gradient(90deg, #e94560, #0f3460, #16c79a);
  transition: width 0.1s linear;
}

/* Animations */
.card[style*="--anim: fade_in"].visible {
  animation: fadeIn 0.6s ease-out;
}
.card[style*="--anim: slide_up"].visible {
  animation: slideUp 0.6s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; transform: scale(0.95); }
  to   { opacity: 1; transform: scale(1); }
}
@keyframes slideUp {
  from { opacity: 0; transform: translateY(40px); }
  to   { opacity: 1; transform: translateY(0); }
}
`
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

func buildHyperFramesIndex(topic, script string) string {
	if topic == "" {
		topic = "Tangying AIOS Video"
	}
	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + html.EscapeString(topic) + `</title>
  <style>
    body { margin: 0; font-family: system-ui, sans-serif; background: #111; color: #fff; }
    main { width: 100vw; height: 100vh; display: grid; place-items: center; padding: 8vw; box-sizing: border-box; }
    h1 { font-size: 7vw; margin: 0 0 3vw; }
    p { font-size: 2.4vw; line-height: 1.55; max-width: 70vw; }
  </style>
</head>
<body>
  <main>
    <section>
      <h1>` + html.EscapeString(topic) + `</h1>
      <p>` + html.EscapeString(script) + `</p>
    </section>
  </main>
</body>
</html>
`
}
