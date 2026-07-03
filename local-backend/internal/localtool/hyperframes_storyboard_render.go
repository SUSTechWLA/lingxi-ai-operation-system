package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type storyboardRenderData struct {
	Script            string                        `json:"script"`
	Topic             string                        `json:"topic"`
	ShotList          []storyboardRenderShot        `json:"shotList"`
	ShotAssetPackages []storyboardRenderAssetBundle `json:"shotAssetPackages"`
}

type storyboardRenderShot struct {
	ID              string  `json:"id"`
	ShotID          string  `json:"shotId"`
	SceneSummary    string  `json:"sceneSummary"`
	MainAction      string  `json:"mainAction"`
	RecommendedMode string  `json:"recommendedMode"`
	DurationSec     float64 `json:"durationSec"`
	SequenceIndex   int     `json:"sequenceIndex"`
}

type storyboardRenderAssetBundle struct {
	ShotID    string                 `json:"shotId"`
	AIGCVideo map[string]interface{} `json:"aigcVideo"`
}

func fastStoryboardRenderEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("TANGYING_FAST_STORYBOARD_RENDER")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func (e *HyperFramesRenderExecutor) renderFastStoryboard(ctx context.Context, projectID, projectDir, outputPath string, fps, width, height int) (*hyperFramesRenderResponse, bool, error) {
	dataPath := filepath.Join(projectDir, "assets", "data.json")
	payload, err := os.ReadFile(dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("read storyboard data: %w", err)
	}

	var data storyboardRenderData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, true, fmt.Errorf("parse storyboard data: %w", err)
	}
	if len(data.ShotList) == 0 {
		return nil, false, nil
	}
	durationSec := storyboardTotalDurationSec(data.ShotList)
	if durationSec <= 0 {
		durationSec = 1
	}
	if width <= 0 {
		width = 1920
	}
	if height <= 0 {
		height = 1080
	}
	if fps <= 0 {
		fps = 30
	}

	workDir := filepath.Join(filepath.Dir(outputPath), "storyboard-fast-render-"+sanitizeRenderSegment(projectID))
	if err := os.RemoveAll(workDir); err != nil {
		return nil, true, fmt.Errorf("clear storyboard render workspace: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, true, fmt.Errorf("create storyboard render workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, true, fmt.Errorf("create render output directory: %w", err)
	}

	renderDataPath := filepath.Join(workDir, "storyboard-data.json")
	if err := os.WriteFile(renderDataPath, payload, 0o644); err != nil {
		return nil, true, fmt.Errorf("write storyboard render data: %w", err)
	}

	scriptPath := filepath.Join(workDir, "render_storyboard.py")
	if err := os.WriteFile(scriptPath, []byte(storyboardRenderPythonScript), 0o755); err != nil {
		return nil, true, fmt.Errorf("write storyboard render script: %w", err)
	}

	pythonPath, err := storyboardPythonPath(ctx)
	if err != nil {
		return nil, true, err
	}
	pythonCmd := exec.CommandContext(ctx, pythonPath, scriptPath, renderDataPath, projectDir, workDir, strconv.Itoa(width), strconv.Itoa(height))
	if combined, err := pythonCmd.CombinedOutput(); err != nil {
		return nil, true, fmt.Errorf("generate storyboard frames: %w: %s", err, strings.TrimSpace(string(combined)))
	}

	concatPath := filepath.Join(workDir, "concat.txt")
	ffmpegArgs := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatPath,
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-vf", fmt.Sprintf("fps=%d,format=yuv420p", fps),
		"-r", strconv.Itoa(fps),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "18",
		"-movflags", "+faststart",
		outputPath,
	}
	ffmpegCmd := exec.CommandContext(ctx, "ffmpeg", ffmpegArgs...)
	if combined, err := ffmpegCmd.CombinedOutput(); err != nil {
		return nil, true, fmt.Errorf("assemble storyboard video: %w: %s", err, strings.TrimSpace(string(combined)))
	}

	return &hyperFramesRenderResponse{
		OK:         true,
		JobID:      "storyboard_fast_render",
		OutputPath: outputPath,
	}, true, nil
}

func storyboardTotalDurationSec(shots []storyboardRenderShot) float64 {
	var total float64
	for _, shot := range shots {
		if shot.DurationSec > 0 {
			total += shot.DurationSec
		}
	}
	return total
}

func sanitizeRenderSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "project"
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
			b.WriteRune('-')
		}
	}
	return b.String()
}

func storyboardPythonPath(ctx context.Context) (string, error) {
	seen := map[string]bool{}
	add := func(candidates *[]string, path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		*candidates = append(*candidates, path)
	}

	var candidates []string
	add(&candidates, os.Getenv("TANGYING_RENDER_PYTHON"))
	if pyenvRoot := os.Getenv("PYENV_ROOT"); pyenvRoot != "" {
		add(&candidates, filepath.Join(pyenvRoot, "shims", "python3"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(&candidates, filepath.Join(home, ".pyenv", "shims", "python3"))
		add(&candidates, filepath.Join(home, ".pyenv", "versions", "3.11.9", "bin", "python3"))
	}
	add(&candidates, "/opt/homebrew/bin/python3")
	add(&candidates, "/usr/local/bin/python3")
	add(&candidates, "python3")

	var errors []string
	for _, candidate := range candidates {
		cmd := exec.CommandContext(ctx, candidate, "-c", "import PIL")
		if err := cmd.Run(); err == nil {
			return candidate, nil
		} else {
			errors = append(errors, candidate+": "+err.Error())
		}
	}
	return "", fmt.Errorf("fast storyboard render requires Python with Pillow; tried %s", strings.Join(errors, "; "))
}

const storyboardRenderPythonScript = `
import json
import math
import os
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont, ImageFilter

data_path = Path(sys.argv[1])
project_dir = Path(sys.argv[2])
out_dir = Path(sys.argv[3])
width = int(sys.argv[4])
height = int(sys.argv[5])

data = json.loads(data_path.read_text(encoding="utf-8"))
shots = data.get("shotList") or []
packages = {pkg.get("shotId"): pkg for pkg in (data.get("shotAssetPackages") or [])}
screens_dir = project_dir / "assets" / "screens"

def font(size, bold=False):
    candidates = [
        "/System/Library/Fonts/PingFang.ttc",
        "/System/Library/Fonts/STHeiti Medium.ttc",
        "/System/Library/Fonts/Supplemental/Songti.ttc",
        "/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
        "/Library/Fonts/Arial Unicode.ttf",
    ]
    for path in candidates:
        if os.path.exists(path):
            try:
                return ImageFont.truetype(path, size=size, index=1 if bold else 0)
            except Exception:
                try:
                    return ImageFont.truetype(path, size=size)
                except Exception:
                    pass
    return ImageFont.load_default()

FONT_HUGE = font(88, True)
FONT_TITLE = font(58, True)
FONT_BODY = font(34)
FONT_SMALL = font(24)
FONT_MONO = font(22)

def text_size(draw, text, fnt):
    box = draw.textbbox((0, 0), text, font=fnt)
    return box[2] - box[0], box[3] - box[1]

def wrap_text(draw, text, fnt, max_width, max_lines):
    text = " ".join(str(text or "").replace("\n", " ").split())
    if not text:
        return []
    lines, current = [], ""
    for ch in text:
        trial = current + ch
        if text_size(draw, trial, fnt)[0] <= max_width or not current:
            current = trial
            continue
        lines.append(current)
        current = ch
        if len(lines) >= max_lines:
            break
    if current and len(lines) < max_lines:
        lines.append(current)
    if len(lines) == max_lines and len("".join(lines)) < len(text):
        lines[-1] = lines[-1].rstrip("，。,. ") + "..."
    return lines

def split_route(summary):
    summary = str(summary or "")
    if "|" in summary:
        left, right = summary.split("|", 1)
        return left.strip(), right.strip()
    return "HYPERFRAMES", summary.strip()

def gradient_bg(seed):
    img = Image.new("RGB", (width, height), "#101820")
    pix = img.load()
    palettes = [
        ((15, 23, 32), (35, 54, 67), (196, 92, 47)),
        ((12, 21, 34), (31, 75, 76), (218, 164, 79)),
        ((18, 20, 38), (73, 55, 109), (60, 149, 142)),
        ((18, 24, 31), (62, 82, 97), (211, 102, 79)),
    ]
    a, b, c = palettes[seed % len(palettes)]
    for y in range(height):
        t = y / max(1, height - 1)
        for x in range(width):
            u = x / max(1, width - 1)
            glow = max(0, 1 - ((u - 0.82) ** 2 + (t - 0.22) ** 2) * 5)
            r = int(a[0] * (1 - t) + b[0] * t + c[0] * glow * 0.34)
            g = int(a[1] * (1 - t) + b[1] * t + c[1] * glow * 0.34)
            bl = int(a[2] * (1 - t) + b[2] * t + c[2] * glow * 0.34)
            pix[x, y] = (min(255, r), min(255, g), min(255, bl))
    return img

def draw_badge(draw, xy, text, fill=(242, 194, 139), outline=(255, 255, 255, 42)):
    x, y = xy
    tw, th = text_size(draw, text, FONT_SMALL)
    rect = (x, y, x + tw + 34, y + th + 22)
    draw.rounded_rectangle(rect, radius=18, fill=(255, 255, 255, 22), outline=outline, width=1)
    draw.text((x + 17, y + 10), text, font=FONT_SMALL, fill=fill)

def draw_browser_frame(draw, shot, idx):
    candidates = [
        screens_dir / f"shot_{idx+1:02d}.png",
        screens_dir / f"shot_{idx+1}.png",
        screens_dir / ("trace.png" if idx == 4 else "artifacts.png"),
    ]
    frame = (880, 185, 1770, 795)
    draw.rounded_rectangle(frame, radius=28, fill=(245, 247, 248), outline=(255, 255, 255), width=2)
    draw.rounded_rectangle((frame[0], frame[1], frame[2], frame[1] + 56), radius=28, fill=(225, 232, 238))
    for i, color in enumerate([(238, 93, 80), (246, 190, 78), (98, 201, 107)]):
        draw.ellipse((frame[0] + 28 + i * 34, frame[1] + 20, frame[0] + 46 + i * 34, frame[1] + 38), fill=color)
    image_path = next((p for p in candidates if p.exists()), None)
    if image_path:
        shot_img = Image.open(image_path).convert("RGB")
        target = (frame[2] - frame[0] - 32, frame[3] - frame[1] - 88)
        shot_img.thumbnail(target, Image.Resampling.LANCZOS)
        canvas = Image.new("RGB", target, (239, 243, 246))
        canvas.paste(shot_img, ((target[0] - shot_img.width) // 2, (target[1] - shot_img.height) // 2))
        img.paste(canvas, (frame[0] + 16, frame[1] + 72))
    else:
        for row in range(5):
            y = frame[1] + 98 + row * 76
            draw.rounded_rectangle((frame[0] + 54, y, frame[2] - 54, y + 42), radius=12, fill=(205, 216, 226))
        draw.text((frame[0] + 70, frame[1] + 270), "真实页面录屏占位", font=FONT_TITLE, fill=(31, 41, 55))

def draw_pipeline(draw):
    labels = ["一句话需求", "脚本", "分镜", "审核门", "MCP", "本地渲染"]
    x0, y0 = 880, 300
    for i, label in enumerate(labels):
        x = x0 + (i % 3) * 280
        y = y0 + (i // 3) * 180
        draw.rounded_rectangle((x, y, x + 220, y + 94), radius=22, fill=(242, 239, 228, 34), outline=(240, 194, 139, 160), width=2)
        draw.text((x + 32, y + 30), label, font=FONT_BODY, fill=(242, 239, 228))
        if i < len(labels) - 1:
            x2 = x0 + ((i + 1) % 3) * 280
            y2 = y0 + ((i + 1) // 3) * 180
            draw.line((x + 220, y + 47, x2, y2 + 47), fill=(240, 194, 139, 160), width=5)

def draw_aigc_reel(draw, shot_id):
    pkg = packages.get(shot_id) or {}
    aigc = pkg.get("aigcVideo") or {}
    submit_id = str(aigc.get("submitId") or aigc.get("submit_id") or "submitted")
    status = str(aigc.get("genStatus") or aigc.get("gen_status") or "querying")
    reel = (910, 180, 1745, 825)
    draw.rounded_rectangle(reel, radius=34, fill=(8, 13, 22, 170), outline=(240, 194, 139, 110), width=2)
    for i in range(4):
        x = reel[0] + 58 + (i % 2) * 374
        y = reel[1] + 68 + (i // 2) * 242
        draw.rounded_rectangle((x, y, x + 310, y + 190), radius=24, fill=(45 + i*22, 83 + i*13, 92 + i*8), outline=(255, 255, 255, 45))
        draw.arc((x + 46, y + 40, x + 210, y + 170), 210, 520, fill=(242, 194, 139), width=8)
        draw.ellipse((x + 198, y + 56, x + 262, y + 120), fill=(200, 107, 52, 180))
    draw_badge(draw, (reel[0] + 54, reel[3] - 112), f"JiMeng MCP: {status}")
    draw.text((reel[0] + 54, reel[3] - 58), submit_id[:36], font=FONT_MONO, fill=(242, 239, 228, 190))

def draw_release_cards(draw):
    items = ["README 更新内容", "Wiki: develop_go 是开发者分支", "合入 release 后写发布说明", "Tag 管理版本"]
    for i, item in enumerate(items):
        x = 900
        y = 220 + i * 135
        draw.rounded_rectangle((x, y, 1740, y + 96), radius=18, fill=(242, 239, 228, 28), outline=(79, 138, 139, 180), width=2)
        draw.text((x + 34, y + 30), item, font=FONT_BODY, fill=(242, 239, 228))
        draw.text((x + 740, y + 30), "OK", font=FONT_BODY, fill=(120, 220, 160))

def draw_common_text(draw, idx, total, route, title, action, duration):
    draw_badge(draw, (92, 78), "躺营 AI Operation System")
    draw.text((92, 150), f"{idx+1:02d} / {total:02d}", font=FONT_MONO, fill=(240, 194, 139))
    draw.text((92, 194), route, font=FONT_BODY, fill=(240, 194, 139))
    for n, line in enumerate(wrap_text(draw, title, FONT_TITLE, 720, 5)):
        draw.text((92, 264 + n * 72), line, font=FONT_TITLE, fill=(242, 239, 228))
    body_y = 650
    for n, line in enumerate(wrap_text(draw, action, FONT_BODY, 700, 3)):
        draw.text((96, body_y + n * 48), line, font=FONT_BODY, fill=(242, 239, 228, 218))
    draw.rounded_rectangle((92, 875, 1770, 916), radius=18, fill=(255, 255, 255, 26))
    draw.rounded_rectangle((92, 875, 92 + int(1678 * ((idx + 1) / max(1, total))), 916), radius=18, fill=(200, 107, 52))
    draw.text((92, 940), f"{duration:.0f}s · 可审核 · 可追踪 · 可本地执行", font=FONT_SMALL, fill=(242, 239, 228, 188))

manifest_lines = []
total = len(shots)
for idx, shot in enumerate(shots):
    global img
    img = gradient_bg(idx)
    overlay = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    for r in range(0, 900, 120):
        draw.ellipse((width - 520 - r//3, 50 + r//5, width + 260 + r//3, 780 + r//5), outline=(255,255,255,18), width=2)
    img = Image.alpha_composite(img.convert("RGBA"), overlay)
    draw = ImageDraw.Draw(img)
    route, title = split_route(shot.get("sceneSummary"))
    action = shot.get("mainAction") or "把复杂流程变成非技术用户也能看懂的画面。"
    duration = float(shot.get("durationSec") or 6)
    route_upper = route.upper()
    if "SCREEN" in route_upper or "录屏" in route:
        draw_browser_frame(draw, shot, idx)
    elif "AIGC" in route_upper or "即梦" in title:
        draw_aigc_reel(draw, shot.get("shotId") or shot.get("id"))
    elif "README" in title.upper() or "WIKI" in title.upper() or "TAG" in title.upper():
        draw_release_cards(draw)
    else:
        draw_pipeline(draw)
    draw_common_text(draw, idx, total, route, title, action, duration)
    out = out_dir / f"shot_{idx+1:03d}.png"
    img.convert("RGB").save(out, "PNG")
    manifest_lines.append(f"file '{out.as_posix()}'\n")
    manifest_lines.append(f"duration {max(1.0, duration):.3f}\n")

if shots:
    last = out_dir / f"shot_{len(shots):03d}.png"
    manifest_lines.append(f"file '{last.as_posix()}'\n")
(out_dir / "concat.txt").write_text("".join(manifest_lines), encoding="utf-8")
`
