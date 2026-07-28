package localtool

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

const productionAudioProvenanceSchema = "tangying-production-audio-provenance/v1"

var productionVoiceMIMETypes = map[string]struct{}{
	"audio/wav":    {},
	"audio/x-wav":  {},
	"audio/mpeg":   {},
	"audio/mp4":    {},
	"audio/x-m4a":  {},
	"audio/flac":   {},
	"audio/x-flac": {},
}

type resolvedProjectVoiceArtifact struct {
	ProjectID   string
	ArtifactID  string
	StorageRef  string
	MIMEType    string
	ContentHash string
	ContentPath string
}

type projectVoiceArtifactMetadata struct {
	ID                   string `json:"id"`
	ProjectID            string `json:"projectId"`
	StorageRef           string `json:"storageRef"`
	MIMEType             string `json:"mimeType"`
	ContentHash          string `json:"contentHash"`
	UsageRightsConfirmed bool   `json:"usageRightsConfirmed"`
}

func normalizeSHA256(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "sha256:")
	if len(normalized) != 64 {
		return "", errors.New("SHA-256 must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(normalized); err != nil {
		return "", errors.New("SHA-256 must contain only hexadecimal characters")
	}
	return normalized, nil
}

func hashFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func requireRegularReadableFile(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s is unreadable: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 {
		return fmt.Errorf("%s must be a nonempty regular file", label)
	}
	if info.Mode().Perm()&0o444 == 0 {
		return fmt.Errorf("%s is not readable", label)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s is unreadable: %w", label, err)
	}
	return file.Close()
}

func resolveProjectVoiceArtifact(
	dataDir, projectID, artifactID, storageRef, expectedHash string,
	allowedMIME map[string]struct{},
) (resolvedProjectVoiceArtifact, error) {
	var empty resolvedProjectVoiceArtifact
	projectID = strings.TrimSpace(projectID)
	artifactID = strings.TrimSpace(artifactID)
	if err := validateLocalSegment(projectID); err != nil {
		return empty, fmt.Errorf("projectId must be a safe path segment: %w", err)
	}
	if err := validateLocalSegment(artifactID); err != nil {
		return empty, fmt.Errorf("artifactId must be a safe path segment: %w", err)
	}
	if strings.TrimSpace(dataDir) == "" {
		return empty, errors.New("local data directory is required")
	}
	prefix := "local://projects/" + projectID + "/artifacts/" + artifactID + "/"
	storageRef = strings.TrimSpace(storageRef)
	if !strings.HasPrefix(storageRef, prefix) || len(storageRef) <= len(prefix) || strings.Contains(storageRef, "..") || strings.ContainsAny(storageRef, "?#") {
		return empty, errors.New("storageRef must identify the requested project-local artifact")
	}
	expectedDigest, err := normalizeSHA256(expectedHash)
	if err != nil {
		return empty, fmt.Errorf("expected content hash is invalid: %w", err)
	}

	artifactDir := filepath.Join(dataDir, "artifacts", projectID, artifactID)
	contentPath := filepath.Join(artifactDir, "content")
	metadataPath := filepath.Join(artifactDir, "metadata.json")
	if err := ensureInside(dataDir, contentPath); err != nil {
		return empty, err
	}
	if err := ensureInside(dataDir, metadataPath); err != nil {
		return empty, err
	}
	if err := requireRegularReadableFile(metadataPath, "artifact metadata"); err != nil {
		return empty, err
	}
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		return empty, fmt.Errorf("read artifact metadata: %w", err)
	}
	var metadata projectVoiceArtifactMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return empty, fmt.Errorf("decode artifact metadata: %w", err)
	}
	if metadata.ProjectID != projectID || metadata.ID != artifactID {
		return empty, errors.New("artifact metadata scope does not match the requested project and artifact")
	}
	if metadata.StorageRef != storageRef {
		return empty, errors.New("artifact metadata storageRef does not match the request")
	}
	mimeType := strings.ToLower(strings.TrimSpace(metadata.MIMEType))
	if _, ok := allowedMIME[mimeType]; !ok {
		return empty, fmt.Errorf("artifact MIME type %q is not an allowed audio type", mimeType)
	}
	if !metadata.UsageRightsConfirmed {
		return empty, errors.New("voice artifact requires explicit usage-rights confirmation")
	}
	metadataDigest, err := normalizeSHA256(metadata.ContentHash)
	if err != nil || metadataDigest != expectedDigest {
		return empty, errors.New("artifact metadata content hash does not match the request")
	}
	if err := requireRegularReadableFile(contentPath, "artifact content"); err != nil {
		return empty, err
	}
	actualDigest, err := hashFileSHA256(contentPath)
	if err != nil {
		return empty, fmt.Errorf("hash artifact content: %w", err)
	}
	if actualDigest != expectedDigest {
		return empty, errors.New("artifact content hash mismatch")
	}
	return resolvedProjectVoiceArtifact{
		ProjectID: projectID, ArtifactID: artifactID, StorageRef: storageRef,
		MIMEType: mimeType, ContentHash: "sha256:" + actualDigest, ContentPath: contentPath,
	}, nil
}

type voiceCommandRunner func(context.Context, []string) (string, error)

type preparedNarration struct {
	AudioPath      string
	OutputDir      string
	ProvenancePath string
}

func runVoiceCommand(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("voice command is required")
	}
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("voice command failed: %w; output: %s", err, trimCommandOutput(output))
	}
	return string(output), nil
}

type productionWAVInfo struct {
	SampleRate uint32
	Channels   uint16
	Bits       uint16
	Frames     uint32
}

func inspectProductionWAV(path string) (productionWAVInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return productionWAVInfo{}, err
	}
	defer file.Close()
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil || string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return productionWAVInfo{}, errors.New("production master is not a valid WAV")
	}
	var info productionWAVInfo
	var blockAlign uint16
	var dataBytes uint32
	for {
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(file, chunkHeader); err != nil {
			break
		}
		size := binary.LittleEndian.Uint32(chunkHeader[4:8])
		switch string(chunkHeader[:4]) {
		case "fmt ":
			chunk := make([]byte, size)
			if _, err := io.ReadFull(file, chunk); err != nil || len(chunk) < 16 {
				return productionWAVInfo{}, errors.New("production master has an invalid fmt chunk")
			}
			if binary.LittleEndian.Uint16(chunk[0:2]) != 1 {
				return productionWAVInfo{}, errors.New("production master must use PCM encoding")
			}
			info.Channels = binary.LittleEndian.Uint16(chunk[2:4])
			info.SampleRate = binary.LittleEndian.Uint32(chunk[4:8])
			blockAlign = binary.LittleEndian.Uint16(chunk[12:14])
			info.Bits = binary.LittleEndian.Uint16(chunk[14:16])
		case "data":
			dataBytes = size
			if _, err := file.Seek(int64(size), io.SeekCurrent); err != nil {
				return productionWAVInfo{}, err
			}
		default:
			if _, err := file.Seek(int64(size), io.SeekCurrent); err != nil {
				return productionWAVInfo{}, err
			}
		}
		if size%2 == 1 {
			_, _ = file.Seek(1, io.SeekCurrent)
		}
	}
	if info.SampleRate != 48000 || info.Channels != 1 || info.Bits != 16 || blockAlign != 2 || dataBytes == 0 {
		return productionWAVInfo{}, errors.New("production master must be 48 kHz mono PCM16 WAV")
	}
	info.Frames = dataBytes / uint32(blockAlign)
	return info, nil
}

var loudnormJSONPattern = regexp.MustCompile(`(?s)\{[^{}]*"input_i"[^{}]*\}`)

func parseProductionLoudness(output string) (map[string]float64, error) {
	matches := loudnormJSONPattern.FindAllString(output, -1)
	for index := len(matches) - 1; index >= 0; index-- {
		var raw map[string]string
		if err := json.Unmarshal([]byte(matches[index]), &raw); err != nil {
			continue
		}
		values := map[string]float64{}
		valid := true
		for source, target := range map[string]string{"input_i": "integratedLufs", "input_tp": "truePeakDbtp", "input_lra": "loudnessRangeLu"} {
			value, err := strconv.ParseFloat(raw[source], 64)
			if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
				valid = false
				break
			}
			values[target] = value
		}
		if valid {
			return values, nil
		}
	}
	return nil, errors.New("ffmpeg did not return valid loudness measurements")
}

func writeAtomicJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".provenance-*.json")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func masterRecordedNarration(
	ctx context.Context,
	dataDir, projectID, runID string,
	source resolvedProjectVoiceArtifact,
	runner voiceCommandRunner,
) (preparedNarration, error) {
	var empty preparedNarration
	if source.ProjectID != projectID {
		return empty, errors.New("recorded narration project does not match the render project")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return empty, fmt.Errorf("invalid projectId: %w", err)
	}
	if err := validateLocalSegment(runID); err != nil {
		return empty, fmt.Errorf("invalid voice run id: %w", err)
	}
	useDefaultRunner := runner == nil
	if useDefaultRunner {
		runner = runVoiceCommand
	}
	ffmpeg := strings.TrimSpace(os.Getenv("TANGYING_FFMPEG_BIN"))
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil && useDefaultRunner {
			return empty, errors.New("ffmpeg is required to master recorded narration")
		}
		if ffmpeg == "" {
			ffmpeg = "ffmpeg"
		}
	}
	outputDir := filepath.Join(dataDir, "projects", projectID, "voice", runID)
	if err := ensureInside(dataDir, outputDir); err != nil {
		return empty, err
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return empty, fmt.Errorf("create voice output directory: %w", err)
	}
	temp, err := os.CreateTemp(outputDir, ".narration-master-*.wav")
	if err != nil {
		return empty, err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return empty, err
	}
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	filter := "highpass=f=55,lowpass=f=18000,acompressor=threshold=-20dB:ratio=2.5:attack=15:release=180:knee=2.5:makeup=1,loudnorm=I=-16:TP=-1.5:LRA=7"
	if _, err := runner(ctx, []string{
		ffmpeg, "-y", "-i", source.ContentPath, "-af", filter,
		"-ar", "48000", "-ac", "1", "-c:a", "pcm_s16le", tempPath,
	}); err != nil {
		return empty, fmt.Errorf("master recorded narration: %w", err)
	}
	wavInfo, err := inspectProductionWAV(tempPath)
	if err != nil {
		return empty, err
	}
	measurementOutput, err := runner(ctx, []string{
		ffmpeg, "-hide_banner", "-nostats", "-i", tempPath,
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=7:print_format=json", "-f", "null", "-",
	})
	if err != nil {
		return empty, fmt.Errorf("measure recorded narration loudness: %w", err)
	}
	loudness, err := parseProductionLoudness(measurementOutput)
	if err != nil {
		return empty, err
	}
	if math.Abs(loudness["integratedLufs"]-(-16.0)) > 0.5 || loudness["truePeakDbtp"] > -1.5 {
		return empty, errors.New("recorded narration master is outside the production loudness target")
	}
	outputHash, err := hashFileSHA256(tempPath)
	if err != nil {
		return empty, err
	}
	masterPath := filepath.Join(outputDir, "narration_master.wav")
	provenancePath := filepath.Join(outputDir, "narration_master.provenance.json")
	if err := os.Rename(tempPath, masterPath); err != nil {
		return empty, fmt.Errorf("publish narration master: %w", err)
	}
	durationSec := float64(wavInfo.Frames) / float64(wavInfo.SampleRate)
	provenance := map[string]interface{}{
		"schemaVersion":        productionAudioProvenanceSchema,
		"sourceMode":           "recorded_narration",
		"provider":             "user_recording",
		"voiceId":              "",
		"inputFileSha256":      strings.TrimPrefix(source.ContentHash, "sha256:"),
		"outputFileSha256":     outputHash,
		"durationSec":          roundFloat(durationSec, 6),
		"usageRightsConfirmed": true,
		"productionReady":      true,
		"mastering": map[string]interface{}{
			"sampleRateHz": 48000, "channels": 1, "sampleFormat": "pcm_s16le",
			"integratedLufs": loudness["integratedLufs"], "truePeakDbtp": loudness["truePeakDbtp"],
			"loudnessRangeLu": loudness["loudnessRangeLu"],
		},
	}
	if err := writeAtomicJSON(provenancePath, provenance); err != nil {
		_ = os.Remove(masterPath)
		return empty, fmt.Errorf("publish narration provenance: %w", err)
	}
	return preparedNarration{AudioPath: masterPath, OutputDir: outputDir, ProvenancePath: provenancePath}, nil
}

func isIPArollRenderTool(providerID, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(providerID), "ip_avatar_3d") &&
		(strings.EqualFold(strings.TrimSpace(toolName), "render_talking_video") ||
			strings.EqualFold(strings.TrimSpace(toolName), "ip_avatar_3d.render_talking_video"))
}

func voiceRunID(projectID, jobID, script string, selection map[string]interface{}) string {
	canonical, _ := json.Marshal(selection)
	digest := sha256.Sum256([]byte(projectID + "\n" + jobID + "\n" + script + "\n" + string(canonical)))
	return "voice_" + hex.EncodeToString(digest[:8])
}

func voiceSelectionString(selection map[string]interface{}, key string) string {
	value, _ := selection[key].(string)
	return strings.TrimSpace(value)
}

func voiceSelectionBool(selection map[string]interface{}, key string) bool {
	value, _ := selection[key].(bool)
	return value
}

func voiceSelectionFloat(selection map[string]interface{}, key string, fallback float64) float64 {
	switch value := selection[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	default:
		return fallback
	}
}

func validatedPreparedSynthesis(outputDir string, result *localmcp.ToolCallResult) (preparedNarration, error) {
	var empty preparedNarration
	if result == nil {
		return empty, errors.New("reference voice synthesis returned no result")
	}
	if result.IsError {
		return empty, fmt.Errorf("reference voice synthesis failed: %s", mcpErrorText(result))
	}
	structured := result.StructuredContent
	if success, exists := structured["success"]; exists && success != true {
		return empty, errors.New("reference voice synthesis did not report success")
	}
	audioPath, _ := structured["audioPath"].(string)
	provenancePath, _ := structured["provenancePath"].(string)
	resolvedOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return empty, err
	}
	expectedAudio := filepath.Join(resolvedOutput, "narration_master.wav")
	expectedProvenance := filepath.Join(resolvedOutput, "narration_master.provenance.json")
	resolvedAudio, err := filepath.Abs(strings.TrimSpace(audioPath))
	if err != nil || resolvedAudio != expectedAudio {
		return empty, errors.New("reference voice synthesis returned an unexpected audio path")
	}
	resolvedProvenance, err := filepath.Abs(strings.TrimSpace(provenancePath))
	if err != nil || resolvedProvenance != expectedProvenance {
		return empty, errors.New("reference voice synthesis returned an unexpected provenance path")
	}
	if err := requireRegularReadableFile(resolvedAudio, "synthesized narration master"); err != nil {
		return empty, err
	}
	if err := requireRegularReadableFile(resolvedProvenance, "synthesized narration provenance"); err != nil {
		return empty, err
	}
	return preparedNarration{AudioPath: resolvedAudio, OutputDir: resolvedOutput, ProvenancePath: resolvedProvenance}, nil
}

func (e *mcpToolCallExecutor) prepareIPArollVoice(
	ctx context.Context,
	client *localmcp.Client,
	projectID string,
	selection map[string]interface{},
	script string,
	runID string,
	renderArgs map[string]interface{},
) (preparedNarration, error) {
	var empty preparedNarration
	mode := strings.ToLower(voiceSelectionString(selection, "mode"))
	if mode == "" {
		mode = "default_ip"
	}
	outputDir := filepath.Join(e.dataDir, "projects", projectID, "voice", runID)
	if err := ensureInside(e.dataDir, outputDir); err != nil {
		return empty, err
	}
	switch mode {
	case "recorded_narration":
		resolved, err := resolveProjectVoiceArtifact(
			e.dataDir,
			projectID,
			voiceSelectionString(selection, "recordedNarrationArtifactId"),
			voiceSelectionString(selection, "recordedNarrationStorageRef"),
			voiceSelectionString(selection, "recordedNarrationContentHash"),
			productionVoiceMIMETypes,
		)
		if err != nil {
			return empty, fmt.Errorf("resolve recorded narration: %w", err)
		}
		return masterRecordedNarration(ctx, e.dataDir, projectID, runID, resolved, e.voiceRunner)
	case "default_ip", "reference_clone":
		provider := voiceSelectionString(selection, "provider")
		if provider == "" {
			provider = "gpt_sovits_local"
		}
		if provider != "gpt_sovits_local" {
			return empty, errors.New("IP A-roll synthesis provider must be gpt_sovits_local")
		}
		synthesisArgs := map[string]interface{}{
			"text": script, "outputDir": outputDir, "mode": mode, "provider": provider,
			"voiceId":              voiceSelectionString(selection, "voiceId"),
			"language":             voiceSelectionString(selection, "language"),
			"speed":                voiceSelectionFloat(selection, "speed", 1.0),
			"characterProfilePath": stringPayload(renderArgs, "characterProfilePath"),
		}
		if synthesisArgs["language"] == "" {
			synthesisArgs["language"] = "zh"
		}
		if mode == "reference_clone" {
			resolved, err := resolveProjectVoiceArtifact(
				e.dataDir,
				projectID,
				voiceSelectionString(selection, "referenceArtifactId"),
				voiceSelectionString(selection, "referenceStorageRef"),
				voiceSelectionString(selection, "referenceContentHash"),
				productionVoiceMIMETypes,
			)
			if err != nil {
				return empty, fmt.Errorf("resolve reference voice: %w", err)
			}
			synthesisArgs["referenceAudioPath"] = resolved.ContentPath
			synthesisArgs["expectedReferenceAudioSha256"] = strings.TrimPrefix(resolved.ContentHash, "sha256:")
			synthesisArgs["referenceText"] = voiceSelectionString(selection, "referenceText")
			synthesisArgs["referenceTextVerified"] = voiceSelectionBool(selection, "referenceTextVerified")
			synthesisArgs["usageRightsConfirmed"] = voiceSelectionBool(selection, "usageRightsConfirmed")
		}
		result, err := client.CallTool(ctx, "ip_avatar_3d.synthesize_reference_voice", synthesisArgs)
		if err != nil {
			return empty, fmt.Errorf("synthesize IP A-roll reference voice: %w", err)
		}
		return validatedPreparedSynthesis(outputDir, result)
	default:
		return empty, fmt.Errorf("unsupported IP A-roll voice mode %q", mode)
	}
}
