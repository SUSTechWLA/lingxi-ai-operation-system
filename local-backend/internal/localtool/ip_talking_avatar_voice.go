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
	if profile.FallbackPolicy == "" {
		profile.FallbackPolicy = "Use uploaded narration audio for production quality; local say voice is only a deterministic preview fallback."
	}
	if profilePath, err := WriteVoiceProfile(input.OutputDir, profile); err != nil {
		return "", "", err
	} else {
		aiffPath := filepath.Join(input.OutputDir, "narration_preview.aiff")
		wavPath := filepath.Join(input.OutputDir, "narration_preview.wav")
		say := b.sayPath
		if say == "" {
			var err error
			say, err = exec.LookPath("say")
			if err != nil {
				return "", profilePath, fmt.Errorf("audioPath is required because macOS say is not available for local preview narration")
			}
		}
		cmd := exec.CommandContext(ctx, say,
			"-v", profile.VoiceName,
			"-r", fmt.Sprintf("%d", profile.SpeakingRate),
			"-o", aiffPath,
			input.Script,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.WriteFile(filepath.Join(input.OutputDir, "narration_preview_error.log"), out, 0o644)
			return "", profilePath, fmt.Errorf("local preview narration failed: %w; output: %s", err, trimCommandOutput(out))
		}
		ffmpeg := b.ffmpegPath
		if ffmpeg == "" {
			var err error
			ffmpeg, err = exec.LookPath("ffmpeg")
			if err != nil {
				return aiffPath, profilePath, nil
			}
		}
		convert := exec.CommandContext(ctx, ffmpeg,
			"-y",
			"-i", aiffPath,
			"-ac", "1",
			"-ar", "44100",
			wavPath,
		)
		if out, err := convert.CombinedOutput(); err != nil {
			_ = os.WriteFile(filepath.Join(input.OutputDir, "narration_preview_convert_error.log"), out, 0o644)
			return aiffPath, profilePath, nil
		}
		_ = os.WriteFile(filepath.Join(input.OutputDir, "narration_preview_notice.txt"), []byte("本地预览口播音频由 macOS say 生成，仅用于动作和口型预览；正式成片建议上传匹配 IP 人设的高质量口播音频。\n"), 0o644)
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
