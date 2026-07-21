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

func storyboardRenderFallbackEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("TANGYING_STORYBOARD_RENDER_FALLBACK")))
	return value == "" || value == "1" || value == "true" || value == "yes" || value == "on"
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
	aRollPath, hasAroll := storyboardArollMediaPath(projectDir)
	ffmpegArgs := storyboardFFmpegArgs(concatPath, aRollPath, outputPath, durationSec, fps, width, height)
	ffmpegCmd := exec.CommandContext(ctx, "ffmpeg", ffmpegArgs...)
	if combined, err := ffmpegCmd.CombinedOutput(); err != nil {
		return nil, true, fmt.Errorf("assemble storyboard video: %w: %s", err, strings.TrimSpace(string(combined)))
	}

	jobID := "storyboard_fast_render"
	if hasAroll {
		jobID = "storyboard_ip_composite"
	}
	return &hyperFramesRenderResponse{
		OK:         true,
		JobID:      jobID,
		OutputPath: outputPath,
	}, true, nil
}

func storyboardArollMediaPath(projectDir string) (string, bool) {
	mediaDir := filepath.Join(projectDir, "assets", "media")
	entries, err := os.ReadDir(mediaDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.Contains(name, "ip_aroll") || (filepath.Ext(name) != ".mp4" && filepath.Ext(name) != ".mov") {
			continue
		}
		candidate := filepath.Join(mediaDir, entry.Name())
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return candidate, true
		}
	}
	return "", false
}

func storyboardFFmpegArgs(concatPath, aRollPath, outputPath string, durationSec float64, fps, width, height int) []string {
	common := []string{
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-r", strconv.Itoa(fps),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "18",
		"-movflags", "+faststart",
	}
	if strings.TrimSpace(aRollPath) == "" {
		args := []string{"-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-vf", fmt.Sprintf("fps=%d,format=yuv420p", fps)}
		return append(append(args, common...), outputPath)
	}
	filter := fmt.Sprintf(
		"[0:v]fps=%d,split=2[bgsrc][fgsrc];"+
			"[bgsrc]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,boxblur=20:2[bg];"+
			"[fgsrc]scale=%d:%d:force_original_aspect_ratio=decrease[fg];"+
			"[bg][fg]overlay=(W-w)/2:(H-h)/2[base];"+
			"[1:v]fps=%d,format=rgba[textfx];"+
			"[base][textfx]overlay=0:0:shortest=1,format=yuv420p[v]",
		fps, width, height, width, height, width, height, fps,
	)
	args := []string{
		"-y", "-stream_loop", "-1", "-i", aRollPath,
		"-f", "concat", "-safe", "0", "-i", concatPath,
		"-filter_complex", filter,
		"-map", "[v]", "-map", "0:a?",
	}
	args = append(args, common...)
	args = append(args, "-c:a", "aac", "-b:a", "192k", outputPath)
	return args
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
media_dir = project_dir / "assets" / "media"
has_ip_aroll = any("ip_aroll" in item.name.lower() and item.suffix.lower() in (".mp4", ".mov") for item in media_dir.glob("*")) if media_dir.exists() else False

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
    draw_badge(draw, (92, 78), "躺营 AI 视频创作助手")
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

def draw_ip_aroll_text_layer(draw, shot, idx, total, duration):
    margin = max(24, int(width * 0.05))
    badge_font = font(max(16, int(width * 0.024)), True)
    title_font_size = max(30, int(width * 0.052))
    body_font_size = max(20, int(width * 0.028))
    small_font_size = max(15, int(width * 0.021))
    title_font = font(title_font_size, True)
    body_font = font(body_font_size)
    small_font = font(small_font_size)

    badges = ["IP A-ROLL", "HYPERFRAMES TEXT", "AIGC READY"]
    cursor_x = margin
    badge_y = margin
    for badge_index, badge in enumerate(badges):
        tw, th = text_size(draw, badge, badge_font)
        fill = (200, 107, 52, 225) if badge_index == 0 else (9, 20, 30, 175)
        draw.rounded_rectangle((cursor_x, badge_y, cursor_x + tw + 28, badge_y + th + 18), radius=14, fill=fill, outline=(255, 255, 255, 80), width=1)
        draw.text((cursor_x + 14, badge_y + 8), badge, font=badge_font, fill=(248, 244, 235, 245))
        cursor_x += tw + 38

    panel_height = min(max(int(height * 0.25), 190), 360)
    panel_top = height - panel_height - margin
    draw.rounded_rectangle((margin, panel_top, width - margin, height - margin), radius=max(20, int(width * 0.025)), fill=(7, 15, 24, 188), outline=(242, 194, 139, 115), width=2)

    screen_text = shot.get("screenText") or []
    if isinstance(screen_text, str):
        screen_text = [screen_text]
    title = next((str(item).strip() for item in screen_text if str(item).strip()), "一个 Shot，三层协同")
    narration = str(shot.get("narrationText") or shot.get("mainAction") or "IP 角色、可控文字与 AIGC 丰富层在同一 Shot 时间窗中协同。")
    text_x = margin + max(22, int(width * 0.035))
    max_text_width = width - text_x - margin - max(22, int(width * 0.035))
    title_y = panel_top + max(22, int(panel_height * 0.12))
    title_lines = wrap_text(draw, title, title_font, max_text_width, 2)
    for line_index, line in enumerate(title_lines):
        draw.text((text_x, title_y + line_index * int(title_font_size * 1.24)), line, font=title_font, fill=(250, 244, 233, 255))

    body_y = title_y + max(1, len(title_lines)) * int(title_font_size * 1.24) + max(8, int(panel_height * 0.025))
    for line_index, line in enumerate(wrap_text(draw, narration, body_font, max_text_width, 2)):
        draw.text((text_x, body_y + line_index * int(body_font_size * 1.35)), line, font=body_font, fill=(235, 237, 232, 225))

    progress_y = height - margin - max(20, int(panel_height * 0.09))
    progress_w = width - margin * 2 - max(44, int(width * 0.07))
    draw.rounded_rectangle((text_x, progress_y, text_x + progress_w, progress_y + 8), radius=4, fill=(255, 255, 255, 60))
    draw.rounded_rectangle((text_x, progress_y, text_x + int(progress_w * ((idx + 1) / max(1, total))), progress_y + 8), radius=4, fill=(218, 142, 77, 245))
    marker = f"SHOT {idx+1:02d}/{total:02d} · {duration:.0f}s · THREE-LAYER CONTRACT"
    draw.text((text_x, progress_y - small_font_size - 8), marker, font=small_font, fill=(242, 194, 139, 230))

manifest_lines = []
total = len(shots)
for idx, shot in enumerate(shots):
    global img
    duration = float(shot.get("durationSec") or 6)
    if has_ip_aroll:
        img = Image.new("RGBA", (width, height), (0, 0, 0, 0))
        draw = ImageDraw.Draw(img)
        draw_ip_aroll_text_layer(draw, shot, idx, total, duration)
        out = out_dir / f"shot_{idx+1:03d}.png"
        img.save(out, "PNG")
        manifest_lines.append(f"file '{out.as_posix()}'\n")
        manifest_lines.append(f"duration {max(1.0, duration):.3f}\n")
        continue
    img = gradient_bg(idx)
    overlay = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    for r in range(0, 900, 120):
        draw.ellipse((width - 520 - r//3, 50 + r//5, width + 260 + r//3, 780 + r//5), outline=(255,255,255,18), width=2)
    img = Image.alpha_composite(img.convert("RGBA"), overlay)
    draw = ImageDraw.Draw(img)
    route, title = split_route(shot.get("sceneSummary"))
    action = shot.get("mainAction") or "把复杂流程变成非技术用户也能看懂的画面。"
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
