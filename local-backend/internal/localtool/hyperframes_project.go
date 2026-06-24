package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

type HyperFramesProjectExecutor struct {
	dataDir string
}

func NewHyperFramesProjectExecutor(dataDir string) *HyperFramesProjectExecutor {
	return &HyperFramesProjectExecutor{dataDir: dataDir}
}

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
	if err := os.MkdirAll(filepath.Join(projectRoot, "assets"), 0o755); err != nil {
		return nil, err
	}

	topic := stringFromPayload(job.Payload, "topic")
	script := stringFromPayload(job.Payload, "script")
	data := map[string]interface{}{
		"topic":        topic,
		"script":       script,
		"shotList":     job.Payload["shotList"],
		"videoPrompts": job.Payload["videoPrompts"],
		"style":        job.Payload["style"],
		"publishCopy":  job.Payload["publishCopy"],
	}
	dataJSON, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(filepath.Join(projectRoot, "assets", "data.json"), dataJSON, 0o644); err != nil {
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

	localRef := "local://projects/" + projectID + "/hyperframes"
	return &Result{Output: map[string]interface{}{
		"projectDir": localRef,
		"entry":      "index.html",
		"files":      []string{localRef + "/index.html", localRef + "/assets/data.json", localRef + "/manifest.json"},
		"summary":    "HyperFrames project generated locally",
	}}, nil
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
