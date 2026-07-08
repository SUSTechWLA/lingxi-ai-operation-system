package localtool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LocalNarrationAudioBuilder struct {
	sayPath    string
	ffmpegPath string
}

func NewLocalNarrationAudioBuilder() *LocalNarrationAudioBuilder {
	return &LocalNarrationAudioBuilder{}
}

func (b *LocalNarrationAudioBuilder) Build(ctx context.Context, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset) (string, string, error) {
	if strings.TrimSpace(input.Script) == "" {
		return "", "", fmt.Errorf("audioPath is required when script is empty")
	}
	if err := os.MkdirAll(input.OutputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create outputDir for narration audio: %w", err)
	}
	profile := effectiveVoiceProfile(input, asset)
	profile.Provider = "macos_say_preview"
	profile.PreviewOnly = true
	prosodyPlan := BuildVoiceProsodyPlan(input.Script, profile)
	profile.Prosody = prosodyPlan.Prosody
	if profile.FallbackPolicy == "" {
		profile.FallbackPolicy = "Use uploaded narration audio for production quality; local segmented say voice is only a deterministic preview fallback."
	}
	if _, err := WriteVoiceProsodyPlan(input.OutputDir, prosodyPlan); err != nil {
		return "", "", err
	}
	if profilePath, err := WriteVoiceProfile(input.OutputDir, profile); err != nil {
		return "", "", err
	} else {
		say := b.sayPath
		if say == "" {
			var err error
			say, err = exec.LookPath("say")
			if err != nil {
				return "", profilePath, fmt.Errorf("audioPath is required because macOS say is not available for local preview narration")
			}
		}
		ffmpeg := b.ffmpegPath
		if ffmpeg == "" {
			var err error
			ffmpeg, err = exec.LookPath("ffmpeg")
			if err != nil {
				aiffPath, err := buildSingleSayPreview(ctx, say, input.OutputDir, profile, input.Script)
				if err != nil {
					return "", profilePath, err
				}
				return aiffPath, profilePath, nil
			}
		}
		wavPath, err := b.buildSegmentedProsodyPreview(ctx, say, ffmpeg, input.OutputDir, prosodyPlan)
		if err != nil {
			_ = os.WriteFile(filepath.Join(input.OutputDir, "narration_preview_prosody_error.log"), []byte(err.Error()), 0o644)
			aiffPath, fallbackErr := buildSingleSayPreview(ctx, say, input.OutputDir, profile, input.Script)
			if fallbackErr != nil {
				return "", profilePath, err
			}
			return aiffPath, profilePath, nil
		}
		_ = os.WriteFile(filepath.Join(input.OutputDir, "narration_preview_notice.txt"), []byte("本地预览口播音频由 macOS say 分段生成，并加入角色化语速、停顿和轻重音计划；它仍仅用于动作和口型预览，正式成片建议上传匹配 IP 人设的高质量口播音频。\n"), 0o644)
		return wavPath, profilePath, nil
	}
}

func WriteVoiceProfile(outputDir string, profile VoiceProfile) (string, error) {
	path := filepath.Join(outputDir, "voice_profile.json")
	if err := writeJSONFile(path, profile); err != nil {
		return "", fmt.Errorf("write voice_profile.json: %w", err)
	}
	return path, nil
}

func WriteVoiceProsodyPlan(outputDir string, plan VoiceProsodyPlan) (string, error) {
	path := filepath.Join(outputDir, "narration_prosody_plan.json")
	if err := writeJSONFile(path, plan); err != nil {
		return "", fmt.Errorf("write narration_prosody_plan.json: %w", err)
	}
	return path, nil
}

func effectiveVoiceProfile(input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset) VoiceProfile {
	profile := VoiceProfile{}
	if asset != nil {
		profile = asset.VoiceProfile
	}
	override := input.VoiceProfile
	if override.Persona != "" {
		profile.Persona = override.Persona
	}
	if override.DisplayName != "" {
		profile.DisplayName = override.DisplayName
	}
	if override.VoiceName != "" {
		profile.VoiceName = override.VoiceName
	}
	if override.Locale != "" {
		profile.Locale = override.Locale
	}
	if override.SpeakingRate > 0 {
		profile.SpeakingRate = override.SpeakingRate
	}
	if override.Tone != "" {
		profile.Tone = override.Tone
	}
	if override.StylePrompt != "" {
		profile.StylePrompt = override.StylePrompt
	}
	if override.Provider != "" {
		profile.Provider = override.Provider
	}
	if override.PreviewOnly {
		profile.PreviewOnly = true
	}
	if override.FallbackPolicy != "" {
		profile.FallbackPolicy = override.FallbackPolicy
	}
	if profile.Persona == "" && asset != nil {
		profile.Persona = asset.CharacterID
	}
	if profile.DisplayName == "" && asset != nil {
		profile.DisplayName = asset.DisplayName
	}
	if profile.VoiceName == "" || profile.SpeakingRate <= 0 || profile.Tone == "" {
		profile = applyCharacterVoiceDefaults(profile)
	}
	if profile.Provider == "" {
		profile.Provider = "local_preview"
	}
	profile = ensureVoiceProsody(profile)
	return profile
}

func applyCharacterVoiceDefaults(profile VoiceProfile) VoiceProfile {
	persona := strings.ToLower(profile.Persona)
	name := strings.ToLower(profile.DisplayName)
	isBobo := strings.Contains(persona, "bobo") || strings.Contains(name, "波波")
	isAster := strings.Contains(persona, "aster") || strings.Contains(name, "阿斯特")
	switch {
	case isBobo:
		if profile.VoiceName == "" {
			profile.VoiceName = firstAvailableSayVoice([]string{"Sandy (中文（中国大陆）)", "Flo (中文（中国大陆）)", "Tingting"})
		}
		if profile.SpeakingRate <= 0 {
			profile.SpeakingRate = 205
		}
		if profile.Locale == "" {
			profile.Locale = "zh_CN"
		}
		if profile.Tone == "" {
			profile.Tone = "young_lively_playful"
		}
		if profile.StylePrompt == "" {
			profile.StylePrompt = "波波声音年轻、生动、活泼，像热情的内容创作搭子，语速偏快但吐字清楚。"
		}
	case isAster:
		if profile.VoiceName == "" {
			profile.VoiceName = firstAvailableSayVoice([]string{"Reed (中文（中国大陆）)", "Eddy (中文（中国大陆）)", "Sinji", "Tingting"})
		}
		if profile.SpeakingRate <= 0 {
			profile.SpeakingRate = 168
		}
		if profile.Locale == "" {
			profile.Locale = "zh_CN"
		}
		if profile.Tone == "" {
			profile.Tone = "calm_precise_scholarly"
		}
		if profile.StylePrompt == "" {
			profile.StylePrompt = "阿斯特声音沉稳、严谨、逻辑性强，适合解释流程、结论和复杂概念。"
		}
	default:
		if profile.VoiceName == "" {
			profile.VoiceName = firstAvailableSayVoice([]string{"Tingting", "Sandy (中文（中国大陆）)", "Meijia"})
		}
		if profile.SpeakingRate <= 0 {
			profile.SpeakingRate = 185
		}
		if profile.Locale == "" {
			profile.Locale = "zh_CN"
		}
		if profile.Tone == "" {
			profile.Tone = "clear_warm"
		}
	}
	return profile
}

func ensureVoiceProsody(profile VoiceProfile) VoiceProfile {
	prosody := profile.Prosody
	if !prosody.Enabled {
		prosody = defaultVoiceProsody(profile)
	}
	if prosody.BaseRate <= 0 {
		prosody.BaseRate = profile.SpeakingRate
	}
	if prosody.MinRate <= 0 {
		prosody.MinRate = maxInt(120, prosody.BaseRate-18)
	}
	if prosody.MaxRate <= 0 {
		prosody.MaxRate = prosody.BaseRate + 18
	}
	if prosody.PauseShortMs <= 0 {
		prosody.PauseShortMs = 120
	}
	if prosody.PauseMediumMs <= 0 {
		prosody.PauseMediumMs = 240
	}
	if prosody.PauseLongMs <= 0 {
		prosody.PauseLongMs = 420
	}
	if prosody.Variability <= 0 {
		prosody.Variability = 0.14
	}
	prosody.Enabled = true
	prosody.SegmentedTTS = true
	profile.Prosody = prosody
	return profile
}

func defaultVoiceProsody(profile VoiceProfile) VoiceProsody {
	persona := strings.ToLower(profile.Persona)
	name := strings.ToLower(profile.DisplayName)
	isBobo := strings.Contains(persona, "bobo") || strings.Contains(name, "波波")
	isAster := strings.Contains(persona, "aster") || strings.Contains(name, "阿斯特")
	baseRate := profile.SpeakingRate
	if baseRate <= 0 {
		baseRate = 185
	}
	switch {
	case isBobo:
		return VoiceProsody{
			Enabled:       true,
			Style:         "lively_rise_fall",
			BaseRate:      baseRate,
			MinRate:       maxInt(130, baseRate-22),
			MaxRate:       baseRate + 24,
			PauseShortMs:  105,
			PauseMediumMs: 230,
			PauseLongMs:   390,
			Variability:   0.2,
			SegmentedTTS:  true,
			Notes: []string{
				"短句略快，重点词前后有停顿",
				"疑问和转折处降低语速，结论处抬起能量",
			},
		}
	case isAster:
		return VoiceProsody{
			Enabled:       true,
			Style:         "calm_logical_cadence",
			BaseRate:      baseRate,
			MinRate:       maxInt(115, baseRate-18),
			MaxRate:       baseRate + 12,
			PauseShortMs:  150,
			PauseMediumMs: 300,
			PauseLongMs:   520,
			Variability:   0.12,
			SegmentedTTS:  true,
			Notes: []string{
				"逻辑词后留出短暂停顿",
				"结论句略慢，保持沉稳可信",
			},
		}
	default:
		return VoiceProsody{
			Enabled:       true,
			Style:         "clear_warm_cadence",
			BaseRate:      baseRate,
			MinRate:       maxInt(120, baseRate-18),
			MaxRate:       baseRate + 18,
			PauseShortMs:  130,
			PauseMediumMs: 260,
			PauseLongMs:   430,
			Variability:   0.14,
			SegmentedTTS:  true,
		}
	}
}

func BuildVoiceProsodyPlan(script string, profile VoiceProfile) VoiceProsodyPlan {
	profile = ensureVoiceProsody(profile)
	segments := splitProsodySegments(script)
	if len(segments) == 0 && strings.TrimSpace(script) != "" {
		segments = []string{strings.TrimSpace(script)}
	}
	ratePattern := []int{0, 7, -5, 4, -3, 9, -7}
	isAster := strings.Contains(strings.ToLower(profile.Persona), "aster") || strings.Contains(strings.ToLower(profile.DisplayName), "阿斯特")
	planned := make([]VoiceProsodySegment, 0, len(segments))
	for i, text := range segments {
		emphasis := hasProsodyEmphasis(text)
		delta := ratePattern[i%len(ratePattern)]
		if emphasis {
			if isAster {
				delta -= 4
			} else {
				delta += 7
			}
		}
		rate := clampInt(profile.Prosody.BaseRate+delta, profile.Prosody.MinRate, profile.Prosody.MaxRate)
		volume := 1.0
		if i%2 == 1 {
			volume += 0.025
		}
		if emphasis {
			volume += 0.045
		}
		planned = append(planned, VoiceProsodySegment{
			Index:        i + 1,
			Text:         text,
			Rate:         rate,
			Volume:       roundFloat(volume, 3),
			PauseAfterMs: pauseAfterSegment(text, profile.Prosody),
			Emphasis:     emphasis,
			Delivery:     deliveryForSegment(text, emphasis, isAster),
		})
	}
	return VoiceProsodyPlan{
		SchemaVersion: "local-ip-talking-avatar-prosody/v1",
		Persona:       profile.Persona,
		DisplayName:   profile.DisplayName,
		VoiceName:     profile.VoiceName,
		Provider:      profile.Provider,
		PreviewOnly:   profile.PreviewOnly,
		Prosody:       profile.Prosody,
		Segments:      planned,
		GeneratedAt:   nowRFC3339(),
		Metadata: map[string]interface{}{
			"notAIGC": true,
			"purpose": "local_preview_humanized_cadence",
		},
	}
}

func splitProsodySegments(script string) []string {
	script = strings.TrimSpace(script)
	if script == "" {
		return nil
	}
	var result []string
	var b strings.Builder
	for _, r := range script {
		b.WriteRune(r)
		switch r {
		case '。', '！', '？', '；', '.', '!', '?', ';', '，', '、', ',', '：', ':':
			if segment := strings.TrimSpace(b.String()); segment != "" {
				result = append(result, segment)
			}
			b.Reset()
		}
	}
	if segment := strings.TrimSpace(b.String()); segment != "" {
		result = append(result, segment)
	}
	return mergeTinyProsodySegments(result)
}

func mergeTinyProsodySegments(segments []string) []string {
	if len(segments) <= 1 {
		return segments
	}
	var merged []string
	for _, segment := range segments {
		if len([]rune(segment)) <= 3 && len(merged) > 0 {
			merged[len(merged)-1] += segment
			continue
		}
		merged = append(merged, segment)
	}
	return merged
}

func hasProsodyEmphasis(text string) bool {
	keywords := []string{"重点", "注意", "第一", "第二", "但是", "所以", "结论", "记住", "开源", "项目", "流程", "工具", "视频", "创作", "！", "!"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func pauseAfterSegment(text string, prosody VoiceProsody) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	last, _ := lastRune(text)
	switch last {
	case '。', '.', '！', '!', '？', '?':
		return prosody.PauseLongMs
	case '；', ';', '：', ':':
		return prosody.PauseMediumMs + (prosody.PauseLongMs-prosody.PauseMediumMs)/2
	case '，', '、', ',':
		return prosody.PauseShortMs
	default:
		return prosody.PauseMediumMs
	}
}

func deliveryForSegment(text string, emphasis, isAster bool) string {
	switch {
	case emphasis && isAster:
		return "slower_precise_emphasis"
	case emphasis:
		return "brighter_emphasis"
	case strings.ContainsAny(text, "？?"):
		return "soft_rising_question"
	case strings.ContainsAny(text, "！!"):
		return "energetic_finish"
	default:
		return "natural"
	}
}

func (b *LocalNarrationAudioBuilder) buildSegmentedProsodyPreview(ctx context.Context, say, ffmpeg, outputDir string, plan VoiceProsodyPlan) (string, error) {
	if len(plan.Segments) == 0 {
		return "", fmt.Errorf("prosody plan has no segments")
	}
	segmentsDir := filepath.Join(outputDir, "narration_segments")
	if err := os.RemoveAll(segmentsDir); err != nil {
		return "", fmt.Errorf("clean narration segments: %w", err)
	}
	if err := os.MkdirAll(segmentsDir, 0o755); err != nil {
		return "", fmt.Errorf("create narration segments: %w", err)
	}
	var concatFiles []string
	for _, segment := range plan.Segments {
		aiffPath := filepath.Join(segmentsDir, fmt.Sprintf("segment_%03d.aiff", segment.Index))
		wavPath := filepath.Join(segmentsDir, fmt.Sprintf("segment_%03d.wav", segment.Index))
		cmd := exec.CommandContext(ctx, say,
			"-v", plan.VoiceName,
			"-r", fmt.Sprintf("%d", segment.Rate),
			"-o", aiffPath,
			segment.Text,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.WriteFile(filepath.Join(outputDir, "narration_preview_error.log"), out, 0o644)
			return "", fmt.Errorf("local preview narration segment %d failed: %w; output: %s", segment.Index, err, trimCommandOutput(out))
		}
		convert := exec.CommandContext(ctx, ffmpeg,
			"-y",
			"-i", aiffPath,
			"-af", fmt.Sprintf("volume=%.3f", segment.Volume),
			"-ac", "1",
			"-ar", "44100",
			"-acodec", "pcm_s16le",
			wavPath,
		)
		if out, err := convert.CombinedOutput(); err != nil {
			_ = os.WriteFile(filepath.Join(outputDir, "narration_preview_convert_error.log"), out, 0o644)
			return "", fmt.Errorf("convert narration segment %d failed: %w; output: %s", segment.Index, err, trimCommandOutput(out))
		}
		concatFiles = append(concatFiles, wavPath)
		if segment.PauseAfterMs > 0 {
			silencePath := filepath.Join(segmentsDir, fmt.Sprintf("silence_%03d.wav", segment.Index))
			if err := generateSilenceWAV(ctx, ffmpeg, silencePath, segment.PauseAfterMs); err != nil {
				return "", err
			}
			concatFiles = append(concatFiles, silencePath)
		}
	}
	concatPath := filepath.Join(segmentsDir, "concat.txt")
	var list strings.Builder
	for _, path := range concatFiles {
		list.WriteString("file '")
		list.WriteString(escapeConcatPath(path))
		list.WriteString("'\n")
	}
	if err := os.WriteFile(concatPath, []byte(list.String()), 0o644); err != nil {
		return "", fmt.Errorf("write narration concat list: %w", err)
	}
	wavPath := filepath.Join(outputDir, "narration_preview.wav")
	concat := exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatPath,
		"-c", "copy",
		wavPath,
	)
	if out, err := concat.CombinedOutput(); err != nil {
		_ = os.WriteFile(filepath.Join(outputDir, "narration_preview_concat_error.log"), out, 0o644)
		return "", fmt.Errorf("concat narration segments failed: %w; output: %s", err, trimCommandOutput(out))
	}
	return wavPath, nil
}

func buildSingleSayPreview(ctx context.Context, say, outputDir string, profile VoiceProfile, script string) (string, error) {
	aiffPath := filepath.Join(outputDir, "narration_preview.aiff")
	cmd := exec.CommandContext(ctx, say,
		"-v", profile.VoiceName,
		"-r", fmt.Sprintf("%d", profile.SpeakingRate),
		"-o", aiffPath,
		script,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.WriteFile(filepath.Join(outputDir, "narration_preview_error.log"), out, 0o644)
		return "", fmt.Errorf("local preview narration failed: %w; output: %s", err, trimCommandOutput(out))
	}
	return aiffPath, nil
}

func generateSilenceWAV(ctx context.Context, ffmpeg, path string, pauseMs int) error {
	if pauseMs <= 0 {
		return nil
	}
	seconds := float64(pauseMs) / 1000
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-f", "lavfi",
		"-i", "anullsrc=r=44100:cl=mono",
		"-t", fmt.Sprintf("%.3f", seconds),
		"-acodec", "pcm_s16le",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("generate silence wav: %w; output: %s", err, trimCommandOutput(out))
	}
	return nil
}

func escapeConcatPath(path string) string {
	return strings.ReplaceAll(path, "'", `'\''`)
}

func lastRune(s string) (rune, bool) {
	var last rune
	ok := false
	for _, r := range s {
		last = r
		ok = true
	}
	return last, ok
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func firstAvailableSayVoice(candidates []string) string {
	voices := cachedSayVoices()
	if len(voices) == 0 {
		return candidates[len(candidates)-1]
	}
	for _, candidate := range candidates {
		for _, line := range voices {
			if strings.HasPrefix(line, candidate+" ") || strings.HasPrefix(line, candidate+"\t") {
				return candidate
			}
		}
	}
	return candidates[len(candidates)-1]
}

func cachedSayVoices() []string {
	say, err := exec.LookPath("say")
	if err != nil {
		return nil
	}
	out, err := exec.Command(say, "-v", "?").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}
