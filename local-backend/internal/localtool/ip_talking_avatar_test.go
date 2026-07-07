package localtool

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalIpTalkingAvatarRenderValidatesRequiredInput(t *testing.T) {
	root := t.TempDir()
	tool := NewLocalIpTalkingAvatarRenderTool(root)

	if _, err := tool.Execute(context.Background(), LocalIpTalkingAvatarRenderInput{
		AudioPath: filepath.Join(root, "narration.wav"),
		OutputDir: filepath.Join(root, "out"),
	}); err == nil || !strings.Contains(err.Error(), "characterId is required") {
		t.Fatalf("missing characterId should return a clear error, got %v", err)
	}

	if _, err := tool.Execute(context.Background(), LocalIpTalkingAvatarRenderInput{
		CharacterID: "demo_ip_001",
		OutputDir:   filepath.Join(root, "out"),
	}); err == nil || !strings.Contains(err.Error(), "audioPath is required") {
		t.Fatalf("missing audioPath should return a clear error, got %v", err)
	}
}

func TestLocalIpTalkingAvatarRenderReportsMissingCharacterAsset(t *testing.T) {
	root := t.TempDir()
	audioPath := writeDemoWAV(t, filepath.Join(root, "input", "demo.wav"), 1.2)
	tool := NewLocalIpTalkingAvatarRenderTool(root)

	_, err := tool.Execute(context.Background(), LocalIpTalkingAvatarRenderInput{
		CharacterID: "unknown_ip",
		AudioPath:   audioPath,
		OutputDir:   filepath.Join(root, "out"),
	})
	if err == nil || !strings.Contains(err.Error(), "character asset not found") {
		t.Fatalf("missing character asset should return a clear error, got %v", err)
	}
}

func TestLocalIpTalkingAvatarRenderGeneratesTimelinesAndScene(t *testing.T) {
	requireFFmpegForAvatarTest(t)
	root := t.TempDir()
	writeDemoCharacter(t, filepath.Join(root, "assets", "characters", "demo_ip_001"))
	audioPath := writeDemoWAV(t, filepath.Join(root, "testdata", "audio", "demo.wav"), 1.5)
	subtitlePath := writeDemoSRT(t, filepath.Join(root, "testdata", "subtitle", "demo.srt"))
	backgroundPath := writeDemoBackground(t, filepath.Join(root, "testdata", "background", "demo.png"))
	outputDir := filepath.Join(root, "tmp", "ip_talking_avatar_demo")

	tool := NewLocalIpTalkingAvatarRenderTool(root)
	output, err := tool.Execute(context.Background(), LocalIpTalkingAvatarRenderInput{
		CharacterID:    "demo_ip_001",
		Script:         "今天我们测试一个本地 IP 数字人口播工具。重点是它不依赖 AIGC。",
		AudioPath:      audioPath,
		SubtitlePath:   subtitlePath,
		BackgroundPath: backgroundPath,
		OutputDir:      outputDir,
		RenderMode:     "sprite2d",
		Resolution:     Resolution{Width: 320, Height: 180},
		FPS:            12,
		MotionPolicy: MotionPolicy{
			AutoBlink:      true,
			AutoBreath:     true,
			SentenceNod:    true,
			KeywordGesture: true,
		},
	})
	if err != nil {
		t.Fatalf("execute local avatar render: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success output, got %#v", output)
	}
	for _, path := range []string{
		filepath.Join(outputDir, "audio_analysis.json"),
		filepath.Join(outputDir, "lip_sync_timeline.json"),
		filepath.Join(outputDir, "motion_timeline.json"),
		filepath.Join(outputDir, "avatar_scene.json"),
		filepath.Join(outputDir, "render_report.json"),
		output.TimelinePath,
		output.AvatarVideoPath,
		output.VideoPath,
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("expected generated file %s, stat=%v size=%d", path, err, fileSize(info))
		}
	}
	if !output.QA.AudioExists || !output.QA.VideoExists || !output.QA.MouthTimelineGenerated || !output.QA.MotionTimelineGenerated || !output.QA.FinalVideoGenerated {
		t.Fatalf("qa flags should confirm generated outputs, got %#v", output.QA)
	}
	if !output.QA.DurationMatched {
		t.Fatalf("final video duration should be close to audio duration, got %#v", output.QA)
	}
}

func TestLocalIpTalkingAvatarRenderGeneratesSVG2DPuppetScene(t *testing.T) {
	requireFFmpegForAvatarTest(t)
	root := t.TempDir()
	writeDemoSVGCharacter(t, filepath.Join(root, "assets", "characters", "bobo"))
	audioPath := writeDemoWAV(t, filepath.Join(root, "testdata", "audio", "bobo.wav"), 1.2)
	outputDir := filepath.Join(root, "tmp", "ip_talking_avatar_bobo")

	tool := NewLocalIpTalkingAvatarRenderTool(root)
	output, err := tool.Execute(context.Background(), LocalIpTalkingAvatarRenderInput{
		CharacterID:      "bobo",
		Script:           "今天波波介绍这个开源视频创作项目。重点是流程清楚，创作更轻松。",
		AudioPath:        audioPath,
		OutputDir:        outputDir,
		RenderMode:       "svg2d",
		InteractionLevel: "expressive",
		Resolution:       Resolution{Width: 320, Height: 180},
		FPS:              12,
		MotionPolicy:     defaultMotionPolicy(),
	})
	if err != nil {
		t.Fatalf("execute svg2d local avatar render: %v", err)
	}
	if !output.Success || output.ScenePath == "" || output.VoiceProfilePath == "" {
		t.Fatalf("expected scene and voice profile output, got %#v", output)
	}
	if !output.QA.ControlRigLoaded || !output.QA.ReferenceSVGLoaded || !output.QA.SceneGenerated {
		t.Fatalf("svg2d qa should confirm rig/svg/scene, got %#v", output.QA)
	}
	for _, path := range []string{output.VideoPath, output.AvatarVideoPath, output.ScenePath, output.VoiceProfilePath} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("expected generated svg2d file %s, stat=%v size=%d", path, err, fileSize(info))
		}
	}
	var scene AvatarScene
	data, err := os.ReadFile(output.ScenePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &scene); err != nil {
		t.Fatal(err)
	}
	if scene.HyperGenControl == nil || scene.VoiceProfile.Tone == "" {
		t.Fatalf("scene should expose hypergen control and voice profile, got %#v", scene)
	}
}

func TestRegisterDefaultExecutorsRegistersLocalIpTalkingAvatarRender(t *testing.T) {
	root := t.TempDir()
	reg := NewRegistry()
	if err := RegisterDefaultExecutors(reg, ExecutorConfig{DataDir: root}); err != nil {
		t.Fatalf("register defaults: %v", err)
	}
	if !reg.CanExecute(CommandLocalIpTalkingAvatarRender) {
		t.Fatalf("default registry should execute %s", CommandLocalIpTalkingAvatarRender)
	}
}

func writeDemoSVGCharacter(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "renderer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "renderer", "bobo_puppet.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><g id="root"><circle id="body" cx="50" cy="50" r="30"/></g></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	rig := map[string]interface{}{
		"schemaVersion": "hypergen-ip-puppet/v1",
		"characterId":   "bobo",
		"parts":         []map[string]interface{}{{"id": "root"}, {"id": "mouth"}, {"id": "rightArm"}},
		"motionChannels": map[string]interface{}{
			"mouthOpen": map[string]interface{}{"range": []float64{0, 1}},
		},
	}
	rigData, err := json.MarshalIndent(rig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "renderer", "rig.json"), rigData, 0o644); err != nil {
		t.Fatal(err)
	}
	doc := map[string]interface{}{
		"characterId":       "bobo",
		"displayName":       "波波",
		"type":              "cartoon_ip",
		"renderer":          "svg2d",
		"defaultExpression": "bright",
		"defaultPose":       "front_talking_float",
		"canvas": map[string]interface{}{
			"width":  320,
			"height": 180,
			"fps":    12,
		},
		"anchor": map[string]interface{}{
			"x":     160,
			"y":     160,
			"scale": 1.0,
		},
		"assets": map[string]string{
			"referenceSvg": "renderer/bobo_puppet.svg",
			"rig":          "renderer/rig.json",
		},
		"voiceProfile": map[string]interface{}{
			"persona":      "bobo",
			"displayName":  "波波",
			"voiceName":    "Tingting",
			"speakingRate": 205,
			"tone":         "young_lively_playful",
		},
		"capabilities": map[string]bool{
			"lipSync":         true,
			"svgPuppet":       true,
			"hypergenControl": true,
		},
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "character.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireFFmpegForAvatarTest(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
}

func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}

func writeDemoCharacter(t *testing.T, root string) {
	t.Helper()
	assets := map[string]string{
		"body":        "base/body.png",
		"head":        "base/head.png",
		"hair":        "base/hair.png",
		"leftArm":     "base/left_arm.png",
		"rightArm":    "base/right_arm.png",
		"eyeOpen":     "eyes/eye_open.png",
		"eyeHalf":     "eyes/eye_half.png",
		"eyeClose":    "eyes/eye_close.png",
		"mouthClosed": "mouths/mouth_closed.png",
		"mouthA":      "mouths/mouth_a.png",
		"mouthO":      "mouths/mouth_o.png",
		"mouthE":      "mouths/mouth_e.png",
		"mouthI":      "mouths/mouth_i.png",
		"mouthU":      "mouths/mouth_u.png",
	}
	for key, rel := range assets {
		writeDemoLayerPNG(t, filepath.Join(root, rel), key)
	}
	doc := map[string]interface{}{
		"characterId":       "demo_ip_001",
		"displayName":       "Demo IP",
		"type":              "cartoon_ip",
		"renderer":          "sprite2d",
		"defaultExpression": "normal",
		"defaultPose":       "front_talking",
		"canvas": map[string]interface{}{
			"width":  320,
			"height": 180,
			"fps":    12,
		},
		"anchor": map[string]interface{}{
			"x":     160,
			"y":     156,
			"scale": 1.0,
		},
		"assets": assets,
		"capabilities": map[string]bool{
			"lipSync":          true,
			"blink":            true,
			"headNod":          true,
			"headShake":        true,
			"simpleGesture":    true,
			"expressionSwitch": true,
		},
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "character.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDemoLayerPNG(t *testing.T, path, key string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	switch key {
	case "body":
		fillEllipse(img, 160, 130, 48, 34, color.RGBA{R: 92, G: 151, B: 245, A: 255})
	case "head":
		fillEllipse(img, 160, 82, 39, 36, color.RGBA{R: 255, G: 213, B: 166, A: 255})
	case "hair":
		fillRect(img, 128, 50, 192, 70, color.RGBA{R: 87, G: 56, B: 35, A: 255})
	case "leftArm":
		fillRect(img, 102, 112, 130, 125, color.RGBA{R: 80, G: 136, B: 223, A: 255})
	case "rightArm":
		fillRect(img, 190, 112, 218, 125, color.RGBA{R: 80, G: 136, B: 223, A: 255})
	case "eyeOpen":
		fillEllipse(img, 145, 78, 4, 6, color.RGBA{R: 50, G: 47, B: 43, A: 255})
		fillEllipse(img, 175, 78, 4, 6, color.RGBA{R: 50, G: 47, B: 43, A: 255})
	case "eyeHalf":
		fillRect(img, 140, 78, 150, 81, color.RGBA{R: 50, G: 47, B: 43, A: 255})
		fillRect(img, 170, 78, 180, 81, color.RGBA{R: 50, G: 47, B: 43, A: 255})
	case "eyeClose":
		fillRect(img, 139, 80, 151, 82, color.RGBA{R: 50, G: 47, B: 43, A: 255})
		fillRect(img, 169, 80, 181, 82, color.RGBA{R: 50, G: 47, B: 43, A: 255})
	case "mouthClosed":
		fillRect(img, 149, 103, 171, 106, color.RGBA{R: 105, G: 51, B: 54, A: 255})
	case "mouthA":
		fillEllipse(img, 160, 104, 13, 9, color.RGBA{R: 127, G: 45, B: 61, A: 255})
	case "mouthO":
		fillEllipse(img, 160, 104, 10, 12, color.RGBA{R: 117, G: 38, B: 60, A: 255})
	case "mouthE":
		fillRect(img, 148, 99, 172, 109, color.RGBA{R: 135, G: 47, B: 66, A: 255})
	case "mouthI":
		fillRect(img, 152, 101, 168, 107, color.RGBA{R: 135, G: 47, B: 66, A: 255})
	case "mouthU":
		fillEllipse(img, 160, 104, 8, 10, color.RGBA{R: 135, G: 47, B: 66, A: 255})
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func writeDemoBackground(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	fillRect(img, 0, 0, 320, 180, color.RGBA{R: 245, G: 250, B: 255, A: 255})
	fillRect(img, 0, 136, 320, 180, color.RGBA{R: 219, G: 238, B: 229, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeDemoSRT(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "1\n00:00:00,000 --> 00:00:01,500\n今天我们测试一个本地 IP 数字人口播工具。\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeDemoWAV(t *testing.T, path string, durationSec float64) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const sampleRate = 16000
	samples := int(durationSec * sampleRate)
	dataBytes := samples * 2
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	write := func(v interface{}) {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Write([]byte("RIFF")); err != nil {
		t.Fatal(err)
	}
	write(uint32(36 + dataBytes))
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		t.Fatal(err)
	}
	write(uint32(16))
	write(uint16(1))
	write(uint16(1))
	write(uint32(sampleRate))
	write(uint32(sampleRate * 2))
	write(uint16(2))
	write(uint16(16))
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	write(uint32(dataBytes))
	for i := 0; i < samples; i++ {
		amp := 0.18
		if i%(sampleRate/3) < sampleRate/12 {
			amp = 0.75
		}
		sample := int16(math.Sin(2*math.Pi*220*float64(i)/sampleRate) * amp * 32767)
		write(sample)
	}
	return path
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func fillEllipse(img *image.RGBA, cx, cy, rx, ry int, c color.RGBA) {
	for y := cy - ry; y <= cy+ry; y++ {
		for x := cx - rx; x <= cx+rx; x++ {
			dx := float64(x-cx) / float64(rx)
			dy := float64(y-cy) / float64(ry)
			if dx*dx+dy*dy <= 1 && image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}
