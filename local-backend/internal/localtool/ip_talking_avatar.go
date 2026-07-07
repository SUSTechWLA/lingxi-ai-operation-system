package localtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LocalIpTalkingAvatarRenderTool struct {
	dataDir string
	guard   *PathGuard
}

func NewLocalIpTalkingAvatarRenderTool(dataDir string) *LocalIpTalkingAvatarRenderTool {
	return &LocalIpTalkingAvatarRenderTool{
		dataDir: dataDir,
		guard:   NewPathGuard(dataDir),
	}
}

func (t *LocalIpTalkingAvatarRenderTool) Execute(ctx context.Context, input LocalIpTalkingAvatarRenderInput) (*LocalIpTalkingAvatarRenderOutput, error) {
	normalized, err := t.normalizeInput(input)
	if err != nil {
		return nil, err
	}
	asset, err := NewIpAssetLoader(t.dataDir).Load(normalized.CharacterID)
	if err != nil {
		return nil, err
	}
	if normalized.RenderMode == "" {
		normalized.RenderMode = asset.Renderer
	}
	if normalized.RenderMode != asset.Renderer {
		return nil, fmt.Errorf("renderMode %q does not match character %q renderer %q", normalized.RenderMode, asset.CharacterID, asset.Renderer)
	}
	if normalized.RenderMode == "live2d" {
		return nil, fmt.Errorf("renderMode live2d is supported in the asset protocol but needs a Live2D Cubism renderer; use svg2d until model3.json rendering is wired")
	}

	voiceProfilePath := ""
	if strings.TrimSpace(normalized.AudioPath) == "" {
		audioPath, profilePath, err := NewLocalNarrationAudioBuilder().Build(ctx, normalized, asset)
		if err != nil {
			return nil, err
		}
		normalized.AudioPath = audioPath
		voiceProfilePath = profilePath
	} else {
		if profilePath, err := WriteVoiceProfile(normalized.OutputDir, effectiveVoiceProfile(normalized, asset)); err == nil {
			voiceProfilePath = profilePath
		}
	}

	audioAnalysis, err := NewAudioAnalyzer().Analyze(ctx, normalized.AudioPath, normalized.OutputDir, normalized.FPS)
	if err != nil {
		return nil, err
	}
	audioAnalysisPath := filepath.Join(normalized.OutputDir, "audio_analysis.json")

	lipTimeline := NewLipSyncTimelineBuilder().Build(audioAnalysis.Frames)
	lipPath, err := WriteLipSyncTimeline(normalized.OutputDir, lipTimeline)
	if err != nil {
		return nil, fmt.Errorf("write lip sync timeline: %w", err)
	}

	motionTimeline := NewMotionTimelineBuilder().Build(normalized.Script, audioAnalysis.DurationSec, normalized.MotionPolicy)
	motionPath, err := WriteMotionTimeline(normalized.OutputDir, motionTimeline)
	if err != nil {
		return nil, fmt.Errorf("write motion timeline: %w", err)
	}

	timelinePath, err := WriteTimelineBundle(normalized.OutputDir, avatarTimelineBundle{
		CharacterID:        normalized.CharacterID,
		DurationSec:        audioAnalysis.DurationSec,
		FPS:                normalized.FPS,
		LipSyncTimeline:    lipTimeline,
		MotionTimeline:     motionTimeline,
		AudioAnalysisPath:  audioAnalysisPath,
		LipSyncPath:        lipPath,
		MotionTimelinePath: motionPath,
		GeneratedAt:        nowRFC3339(),
	})
	if err != nil {
		return nil, fmt.Errorf("write avatar timeline bundle: %w", err)
	}

	scene := NewAvatarSceneBuilder().Build(normalized, asset, audioAnalysis.DurationSec, audioAnalysisPath, lipPath, motionPath)
	scenePath, err := WriteAvatarScene(normalized.OutputDir, scene)
	if err != nil {
		return nil, fmt.Errorf("write avatar scene: %w", err)
	}

	framesDir, avatarPath, err := renderAvatarLayer(ctx, normalized.OutputDir, normalized, asset, lipTimeline, motionTimeline, audioAnalysis.DurationSec)
	if err != nil {
		return nil, err
	}

	finalPath, err := NewFfmpegComposer().Compose(ctx, framesDir, normalized, audioAnalysis.DurationSec)
	if err != nil {
		return nil, err
	}

	output := LocalIpTalkingAvatarRenderOutput{
		Success:          true,
		CharacterID:      normalized.CharacterID,
		VideoPath:        finalPath,
		AvatarVideoPath:  avatarPath,
		TimelinePath:     timelinePath,
		ScenePath:        scenePath,
		SubtitlePath:     normalized.SubtitlePath,
		VoiceProfilePath: voiceProfilePath,
		DurationSec:      audioAnalysis.DurationSec,
	}
	qa, err := NewRenderQualityChecker().Check(ctx, normalized, output, audioAnalysis.DurationSec, lipPath, motionPath, asset)
	if err != nil {
		return nil, err
	}
	output.QA = qa
	return &output, nil
}

func (t *LocalIpTalkingAvatarRenderTool) normalizeInput(input LocalIpTalkingAvatarRenderInput) (LocalIpTalkingAvatarRenderInput, error) {
	input.CharacterID = strings.TrimSpace(input.CharacterID)
	if input.CharacterID == "" {
		return input, fmt.Errorf("characterId is required")
	}
	input.Script = strings.TrimSpace(input.Script)
	if strings.TrimSpace(input.AudioPath) == "" && input.Script == "" {
		return input, fmt.Errorf("audioPath is required unless script is provided for local preview narration")
	}
	if input.OutputDir == "" {
		input.OutputDir = filepath.Join(t.dataDir, "tmp", "local_ip_talking_avatar_render", time.Now().UTC().Format("20060102T150405Z"))
	}
	if input.RenderMode != "" && input.RenderMode != "sprite2d" && input.RenderMode != "svg2d" && input.RenderMode != "live2d" {
		return input, fmt.Errorf("unsupported renderMode %q; supported: sprite2d, svg2d, reserved: live2d", input.RenderMode)
	}
	if input.InteractionLevel == "" {
		input.InteractionLevel = "expressive"
	}
	if input.Resolution.Width <= 0 {
		input.Resolution.Width = 1920
	}
	if input.Resolution.Height <= 0 {
		input.Resolution.Height = 1080
	}
	if input.FPS <= 0 {
		input.FPS = 30
	}
	if isZeroRenderStyle(input.Style) {
		input.Style = defaultRenderStyle()
	} else {
		if input.Style.Position == "" {
			input.Style.Position = "center_bottom"
		}
		if input.Style.Scale <= 0 {
			input.Style.Scale = 1
		}
	}
	if isZeroMotionPolicy(input.MotionPolicy) {
		input.MotionPolicy = defaultMotionPolicy()
	}

	var err error
	if input.AudioPath != "" {
		input.AudioPath, err = t.resolveReadablePath(input.AudioPath, "audioPath")
		if err != nil {
			return input, err
		}
	}
	if input.SubtitlePath != "" {
		input.SubtitlePath, err = t.resolveReadablePath(input.SubtitlePath, "subtitlePath")
		if err != nil {
			return input, err
		}
	}
	if input.BackgroundPath != "" {
		input.BackgroundPath, err = t.resolveReadablePath(input.BackgroundPath, "backgroundPath")
		if err != nil {
			return input, err
		}
	}
	if input.BgmPath != "" {
		input.BgmPath, err = t.resolveReadablePath(input.BgmPath, "bgmPath")
		if err != nil {
			return input, err
		}
	}
	input.OutputDir, err = t.resolveWritableDir(input.OutputDir)
	if err != nil {
		return input, err
	}
	if err := os.MkdirAll(input.OutputDir, 0o755); err != nil {
		return input, fmt.Errorf("create outputDir: %w", err)
	}
	return input, nil
}

func renderAvatarLayer(ctx context.Context, outputDir string, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, lips []LipSyncFrame, motions []MotionEvent, durationSec float64) (string, string, error) {
	switch input.RenderMode {
	case "sprite2d":
		return NewSprite2DRenderer().Render(ctx, outputDir, input, asset, lips, motions, durationSec)
	case "svg2d":
		return NewSVG2DRenderer().Render(ctx, outputDir, input, asset, lips, motions, durationSec)
	default:
		return "", "", fmt.Errorf("renderMode %q is not implemented", input.RenderMode)
	}
}

func (t *LocalIpTalkingAvatarRenderTool) resolveReadablePath(raw, label string) (string, error) {
	path, err := t.resolvePath(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", label, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s not readable: %w", label, err)
	}
	if info.IsDir() || info.Size() == 0 {
		return "", fmt.Errorf("%s is not a readable file: %s", label, path)
	}
	return path, nil
}

func (t *LocalIpTalkingAvatarRenderTool) resolveWritableDir(raw string) (string, error) {
	path, err := t.resolvePath(raw)
	if err != nil {
		return "", fmt.Errorf("invalid outputDir: %w", err)
	}
	return path, nil
}

func (t *LocalIpTalkingAvatarRenderTool) resolvePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(raw, "local://") {
		return t.guard.ResolveLocalURI(raw)
	}
	if filepath.IsAbs(raw) {
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(raw, prefix) {
				return "", fmt.Errorf("access to %s is forbidden", raw)
			}
		}
		return filepath.Clean(raw), nil
	}
	return filepath.Join(t.dataDir, raw), nil
}

func isZeroRenderStyle(style RenderStyle) bool {
	return style.Position == "" && style.Scale == 0 && !style.SubtitleEnabled && !style.BackgroundEnabled && !style.TransparentAvatarVideo
}

func isZeroMotionPolicy(policy MotionPolicy) bool {
	return !policy.AutoBlink && !policy.AutoBreath && !policy.SentenceNod && !policy.KeywordGesture
}

type localIpTalkingAvatarRenderExecutor struct {
	tool *LocalIpTalkingAvatarRenderTool
}

func NewLocalIpTalkingAvatarRenderExecutor(dataDir string) Executor {
	return &localIpTalkingAvatarRenderExecutor{tool: NewLocalIpTalkingAvatarRenderTool(dataDir)}
}

func (e *localIpTalkingAvatarRenderExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	input := avatarInputFromPayload(job.Payload)
	if input.OutputDir == "" {
		projectID := safeProjectID(job.ProjectID)
		input.OutputDir = filepath.Join(e.tool.dataDir, "projects", projectID, "renders", "local_ip_talking_avatar")
	}
	output, err := e.tool.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	projectID := safeProjectID(job.ProjectID)
	artifacts, err := e.localArtifactsForOutput(projectID, output)
	if err != nil {
		return nil, err
	}
	return &Result{Output: map[string]interface{}{
		"success":          output.Success,
		"summary":          "本地 IP 数字人口播渲染完成",
		"characterId":      output.CharacterID,
		"videoPath":        output.VideoPath,
		"avatarVideoPath":  output.AvatarVideoPath,
		"timelinePath":     output.TimelinePath,
		"scenePath":        output.ScenePath,
		"subtitlePath":     output.SubtitlePath,
		"voiceProfilePath": output.VoiceProfilePath,
		"durationSec":      output.DurationSec,
		"qa":               output.QA,
		"output":           output,
		"artifacts":        artifacts,
		"metrics": map[string]interface{}{
			"durationSec": output.DurationSec,
			"fps":         input.FPS,
			"width":       input.Resolution.Width,
			"height":      input.Resolution.Height,
			"notAIGC":     true,
		},
	}}, nil
}

func (e *localIpTalkingAvatarRenderExecutor) localArtifactsForOutput(projectID string, output *LocalIpTalkingAvatarRenderOutput) ([]map[string]interface{}, error) {
	var artifacts []map[string]interface{}
	final, err := mirrorLocalToolArtifact(e.tool.dataDir, projectID, "local-ip-talking-avatar-final", output.VideoPath, "final.mp4", "video/mp4", map[string]interface{}{
		"characterId": output.CharacterID,
		"durationSec": output.DurationSec,
		"qa":          output.QA,
	})
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, map[string]interface{}{
		"unitId":         "local-ip-talking-avatar-final",
		"kind":           "VIDEO",
		"name":           "final.mp4",
		"storageType":    "local",
		"storageRef":     final.StorageRef,
		"mimeType":       "video/mp4",
		"sizeBytes":      final.SizeBytes,
		"status":         "valid",
		"humanApproved":  false,
		"producedByTool": "local_ip_talking_avatar_render",
		"producedByRole": "本地 IP 数字人口播渲染",
		"metadata":       final.Metadata,
	})
	if output.AvatarVideoPath != "" {
		mime := "video/webm"
		filename := "avatar_layer.webm"
		if strings.HasSuffix(strings.ToLower(output.AvatarVideoPath), ".mp4") {
			mime = "video/mp4"
			filename = "avatar_layer.mp4"
		}
		avatar, err := mirrorLocalToolArtifact(e.tool.dataDir, projectID, "local-ip-talking-avatar-layer", output.AvatarVideoPath, filename, mime, map[string]interface{}{
			"characterId": output.CharacterID,
			"transparent": true,
		})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":         "local-ip-talking-avatar-layer",
			"kind":           "AVATAR_LAYER_VIDEO",
			"name":           filename,
			"storageType":    "local",
			"storageRef":     avatar.StorageRef,
			"mimeType":       mime,
			"sizeBytes":      avatar.SizeBytes,
			"status":         "valid",
			"humanApproved":  false,
			"producedByTool": "local_ip_talking_avatar_render",
			"producedByRole": "本地 IP 数字人口播渲染",
			"metadata":       avatar.Metadata,
		})
	}
	if output.TimelinePath != "" {
		timeline, err := mirrorLocalToolArtifact(e.tool.dataDir, projectID, "local-ip-talking-avatar-timeline", output.TimelinePath, "avatar_timeline.json", "application/json", map[string]interface{}{
			"characterId": output.CharacterID,
			"durationSec": output.DurationSec,
		})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":         "local-ip-talking-avatar-timeline",
			"kind":           "IP_TALKING_AVATAR_TIMELINE",
			"name":           "avatar_timeline.json",
			"storageType":    "local",
			"storageRef":     timeline.StorageRef,
			"mimeType":       "application/json",
			"sizeBytes":      timeline.SizeBytes,
			"status":         "valid",
			"humanApproved":  false,
			"producedByTool": "local_ip_talking_avatar_render",
			"producedByRole": "本地 IP 数字人口播渲染",
			"metadata":       timeline.Metadata,
		})
	}
	if output.ScenePath != "" {
		scene, err := mirrorLocalToolArtifact(e.tool.dataDir, projectID, "local-ip-talking-avatar-scene", output.ScenePath, "avatar_scene.json", "application/json", map[string]interface{}{
			"characterId": output.CharacterID,
			"durationSec": output.DurationSec,
			"purpose":     "hypergen_control_schema",
		})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":         "local-ip-talking-avatar-scene",
			"kind":           "IP_TALKING_AVATAR_SCENE",
			"name":           "avatar_scene.json",
			"storageType":    "local",
			"storageRef":     scene.StorageRef,
			"mimeType":       "application/json",
			"sizeBytes":      scene.SizeBytes,
			"status":         "valid",
			"humanApproved":  false,
			"producedByTool": "local_ip_talking_avatar_render",
			"producedByRole": "本地 IP 数字人口播渲染",
			"metadata":       scene.Metadata,
		})
	}
	if output.VoiceProfilePath != "" {
		voice, err := mirrorLocalToolArtifact(e.tool.dataDir, projectID, "local-ip-talking-avatar-voice-profile", output.VoiceProfilePath, "voice_profile.json", "application/json", map[string]interface{}{
			"characterId": output.CharacterID,
			"durationSec": output.DurationSec,
			"purpose":     "character_voice_profile",
		})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":         "local-ip-talking-avatar-voice-profile",
			"kind":           "IP_TALKING_AVATAR_VOICE_PROFILE",
			"name":           "voice_profile.json",
			"storageType":    "local",
			"storageRef":     voice.StorageRef,
			"mimeType":       "application/json",
			"sizeBytes":      voice.SizeBytes,
			"status":         "valid",
			"humanApproved":  false,
			"producedByTool": "local_ip_talking_avatar_render",
			"producedByRole": "本地 IP 数字人口播渲染",
			"metadata":       voice.Metadata,
		})
	}
	reportPath := filepath.Join(filepath.Dir(output.VideoPath), "render_report.json")
	if fileExists(reportPath) {
		report, err := mirrorLocalToolArtifact(e.tool.dataDir, projectID, "local-ip-talking-avatar-report", reportPath, "render_report.json", "application/json", map[string]interface{}{
			"characterId": output.CharacterID,
			"durationSec": output.DurationSec,
		})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":         "local-ip-talking-avatar-report",
			"kind":           "RENDER_REPORT",
			"name":           "render_report.json",
			"storageType":    "local",
			"storageRef":     report.StorageRef,
			"mimeType":       "application/json",
			"sizeBytes":      report.SizeBytes,
			"status":         "valid",
			"humanApproved":  false,
			"producedByTool": "local_ip_talking_avatar_render",
			"producedByRole": "本地 IP 数字人口播渲染",
			"metadata":       report.Metadata,
		})
	}
	return artifacts, nil
}

type mirroredLocalArtifact struct {
	StorageRef string
	SizeBytes  int64
	Metadata   map[string]interface{}
}

func mirrorLocalToolArtifact(dataDir, projectID, unitID, sourcePath, filename, mimeType string, extra map[string]interface{}) (*mirroredLocalArtifact, error) {
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("invalid project id: %w", err)
	}
	if err := validateLocalSegment(unitID); err != nil {
		return nil, fmt.Errorf("invalid artifact id: %w", err)
	}
	contentPath := filepath.Join(dataDir, "artifacts", projectID, unitID, "content")
	metadataPath := filepath.Join(dataDir, "artifacts", projectID, unitID, "metadata.json")
	if err := ensureInside(dataDir, contentPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open artifact source %s: %w", sourcePath, err)
	}
	defer source.Close()
	target, err := os.Create(contentPath)
	if err != nil {
		return nil, fmt.Errorf("create artifact content: %w", err)
	}
	hasher := sha256.New()
	sizeBytes, copyErr := io.Copy(io.MultiWriter(target, hasher), source)
	closeErr := target.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("copy artifact source: %w", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close artifact content: %w", closeErr)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	storageRef := "local://projects/" + projectID + "/artifacts/" + unitID + "/" + hash + "/" + filename
	metadata := map[string]interface{}{
		"id":            unitID,
		"projectId":     projectID,
		"storageRef":    storageRef,
		"mimeType":      mimeType,
		"contentHash":   "sha256:" + hash,
		"sizeBytes":     sizeBytes,
		"sourcePath":    sourcePath,
		"schemaVersion": 1,
		"sourceType":    "local_ip_talking_avatar_render",
		"providerName":  "local-ip-talking-avatar-renderer",
		"generatedAt":   nowRFC3339(),
		"updatedAt":     time.Now().UTC().Format(time.RFC3339Nano),
		"notAIGC":       true,
	}
	for key, value := range extra {
		metadata[key] = value
	}
	if err := writeLocalToolJSON(metadataPath, metadata); err != nil {
		return nil, fmt.Errorf("write artifact metadata: %w", err)
	}
	return &mirroredLocalArtifact{StorageRef: storageRef, SizeBytes: sizeBytes, Metadata: metadata}, nil
}

func safeProjectID(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "local-ip-avatar-demo"
	}
	if err := validateLocalSegment(projectID); err != nil {
		return "local-ip-avatar-demo"
	}
	return projectID
}

func avatarInputFromPayload(payload map[string]interface{}) LocalIpTalkingAvatarRenderInput {
	input := LocalIpTalkingAvatarRenderInput{
		CharacterID:      stringFromPayload(payload, "characterId"),
		Script:           stringFromPayload(payload, "script"),
		AudioPath:        stringFromPayload(payload, "audioPath"),
		SubtitlePath:     stringFromPayload(payload, "subtitlePath"),
		BackgroundPath:   stringFromPayload(payload, "backgroundPath"),
		BgmPath:          stringFromPayload(payload, "bgmPath"),
		OutputDir:        stringFromPayload(payload, "outputDir"),
		RenderMode:       stringFromPayload(payload, "renderMode"),
		InteractionLevel: stringFromPayload(payload, "interactionLevel"),
	}
	input.Resolution = resolutionFromPayload(payload)
	input.FPS = intFromPayload(payload, "fps")
	input.Style = styleFromPayload(payload)
	input.MotionPolicy = motionPolicyFromPayload(payload)
	input.VoiceProfile = voiceProfileFromPayload(payload)
	return input
}

func resolutionFromPayload(payload map[string]interface{}) Resolution {
	res := avatarMapFromPayload(payload, "resolution")
	return Resolution{
		Width:  avatarIntFromMap(res, "width"),
		Height: avatarIntFromMap(res, "height"),
	}
}

func styleFromPayload(payload map[string]interface{}) RenderStyle {
	style := avatarMapFromPayload(payload, "style")
	return RenderStyle{
		Position:               avatarStringFromMap(style, "position"),
		Scale:                  floatFromAvatarMap(style, "scale"),
		SubtitleEnabled:        avatarBoolFromMap(style, "subtitleEnabled"),
		BackgroundEnabled:      avatarBoolFromMap(style, "backgroundEnabled"),
		TransparentAvatarVideo: avatarBoolFromMap(style, "transparentAvatarVideo"),
	}
}

func motionPolicyFromPayload(payload map[string]interface{}) MotionPolicy {
	policy := avatarMapFromPayload(payload, "motionPolicy")
	return MotionPolicy{
		AutoBlink:      avatarBoolFromMap(policy, "autoBlink"),
		AutoBreath:     avatarBoolFromMap(policy, "autoBreath"),
		SentenceNod:    avatarBoolFromMap(policy, "sentenceNod"),
		KeywordGesture: avatarBoolFromMap(policy, "keywordGesture"),
	}
}

func voiceProfileFromPayload(payload map[string]interface{}) VoiceProfile {
	voice := avatarMapFromPayload(payload, "voiceProfile")
	return VoiceProfile{
		Persona:        avatarStringFromMap(voice, "persona"),
		DisplayName:    avatarStringFromMap(voice, "displayName"),
		VoiceName:      avatarStringFromMap(voice, "voiceName"),
		Locale:         avatarStringFromMap(voice, "locale"),
		SpeakingRate:   avatarIntFromMap(voice, "speakingRate"),
		Tone:           avatarStringFromMap(voice, "tone"),
		StylePrompt:    avatarStringFromMap(voice, "stylePrompt"),
		Provider:       avatarStringFromMap(voice, "provider"),
		PreviewOnly:    avatarBoolFromMap(voice, "previewOnly"),
		FallbackPolicy: avatarStringFromMap(voice, "fallbackPolicy"),
	}
}

func avatarMapFromPayload(payload map[string]interface{}, key string) map[string]interface{} {
	if payload == nil {
		return nil
	}
	if value, ok := payload[key].(map[string]interface{}); ok {
		return value
	}
	return nil
}

func avatarStringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func intFromPayload(payload map[string]interface{}, key string) int {
	if payload == nil {
		return 0
	}
	return avatarIntFromInterface(payload[key])
}

func avatarIntFromMap(values map[string]interface{}, key string) int {
	if values == nil {
		return 0
	}
	return avatarIntFromInterface(values[key])
}

func avatarIntFromInterface(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case jsonNumber:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}

func floatFromAvatarMap(values map[string]interface{}, key string) float64 {
	if values == nil {
		return 0
	}
	switch v := values[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func avatarBoolFromMap(values map[string]interface{}, key string) bool {
	if values == nil {
		return false
	}
	if value, ok := values[key].(bool); ok {
		return value
	}
	return false
}
