package localtool

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectVoiceFixture(t *testing.T, dataDir, projectID, artifactID, mimeType string, authorized bool) (string, string) {
	t.Helper()
	content := []byte("project voice fixture")
	digest := sha256.Sum256(content)
	contentHash := "sha256:" + hex.EncodeToString(digest[:])
	storageRef := "local://projects/" + projectID + "/artifacts/" + artifactID + "/" + hex.EncodeToString(digest[:8]) + "/voice.wav"
	dir := filepath.Join(dataDir, "artifacts", projectID, artifactID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "content"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(map[string]interface{}{
		"id": artifactID, "projectId": projectID, "storageRef": storageRef,
		"mimeType": mimeType, "contentHash": contentHash,
		"usageRightsConfirmed": authorized,
	})
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	return storageRef, contentHash
}

func TestResolveProjectVoiceArtifactAcceptsAuthorizedAudio(t *testing.T) {
	allowed := map[string]struct{}{
		"audio/wav": {}, "audio/mpeg": {}, "audio/mp4": {}, "audio/flac": {},
	}
	for _, mimeType := range []string{"audio/wav", "audio/mpeg", "audio/mp4", "audio/flac"} {
		t.Run(mimeType, func(t *testing.T) {
			dataDir := t.TempDir()
			storageRef, contentHash := writeProjectVoiceFixture(t, dataDir, "project_001", "voice_001", mimeType, true)
			resolved, err := resolveProjectVoiceArtifact(dataDir, "project_001", "voice_001", storageRef, contentHash, allowed)
			if err != nil {
				t.Fatalf("resolveProjectVoiceArtifact() error = %v", err)
			}
			if resolved.ContentHash != contentHash || resolved.MIMEType != mimeType {
				t.Fatalf("resolved = %#v", resolved)
			}
			if resolved.ContentPath != filepath.Join(dataDir, "artifacts", "project_001", "voice_001", "content") {
				t.Fatalf("ContentPath = %q", resolved.ContentPath)
			}
		})
	}
}

func TestResolveProjectVoiceArtifactRejectsUntrustedInputs(t *testing.T) {
	allowed := map[string]struct{}{"audio/wav": {}}
	tests := []struct {
		name       string
		projectID  string
		artifact   string
		mutate     func(string, string) (string, string)
		authorized bool
		mimeType   string
	}{
		{name: "project traversal", projectID: "../other", artifact: "voice_001", authorized: true, mimeType: "audio/wav"},
		{name: "artifact traversal", projectID: "project_001", artifact: "../voice", authorized: true, mimeType: "audio/wav"},
		{name: "remote ref", projectID: "project_001", artifact: "voice_001", authorized: true, mimeType: "audio/wav", mutate: func(_ string, hash string) (string, string) { return "https://example.com/voice.wav", hash }},
		{name: "absolute ref", projectID: "project_001", artifact: "voice_001", authorized: true, mimeType: "audio/wav", mutate: func(_ string, hash string) (string, string) { return "/tmp/voice.wav", hash }},
		{name: "wrong ref project", projectID: "project_001", artifact: "voice_001", authorized: true, mimeType: "audio/wav", mutate: func(ref, hash string) (string, string) {
			return strings.Replace(ref, "projects/project_001", "projects/other", 1), hash
		}},
		{name: "wrong ref artifact", projectID: "project_001", artifact: "voice_001", authorized: true, mimeType: "audio/wav", mutate: func(ref, hash string) (string, string) {
			return strings.Replace(ref, "artifacts/voice_001", "artifacts/other", 1), hash
		}},
		{name: "wrong expected hash", projectID: "project_001", artifact: "voice_001", authorized: true, mimeType: "audio/wav", mutate: func(ref, _ string) (string, string) { return ref, "sha256:" + strings.Repeat("0", 64) }},
		{name: "missing authorization", projectID: "project_001", artifact: "voice_001", authorized: false, mimeType: "audio/wav"},
		{name: "non audio mime", projectID: "project_001", artifact: "voice_001", authorized: true, mimeType: "video/mp4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			fixtureProject := "project_001"
			fixtureArtifact := "voice_001"
			ref, hash := writeProjectVoiceFixture(t, dataDir, fixtureProject, fixtureArtifact, tc.mimeType, tc.authorized)
			if tc.mutate != nil {
				ref, hash = tc.mutate(ref, hash)
			}
			if _, err := resolveProjectVoiceArtifact(dataDir, tc.projectID, tc.artifact, ref, hash, allowed); err == nil {
				t.Fatal("resolveProjectVoiceArtifact() error = nil")
			}
		})
	}
}

func writePCM16WAVForVoiceTest(t *testing.T, path string, frames uint32) {
	t.Helper()
	dataSize := frames * 2
	buf := make([]byte, 44+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], 36+dataSize)
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint16(buf[22:24], 1)
	binary.LittleEndian.PutUint32(buf[24:28], 48000)
	binary.LittleEndian.PutUint32(buf[28:32], 96000)
	binary.LittleEndian.PutUint16(buf[32:34], 2)
	binary.LittleEndian.PutUint16(buf[34:36], 16)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedNarrationMasterPublishesVerifiedProductionAudio(t *testing.T) {
	dataDir := t.TempDir()
	ref, hash := writeProjectVoiceFixture(t, dataDir, "project_001", "narration_001", "audio/wav", true)
	resolved, err := resolveProjectVoiceArtifact(dataDir, "project_001", "narration_001", ref, hash, productionVoiceMIMETypes)
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	runner := func(_ context.Context, args []string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			writePCM16WAVForVoiceTest(t, args[len(args)-1], 48000)
			return "", nil
		}
		return `{"input_i":"-16.0","input_tp":"-1.7","input_lra":"2.4"}`, nil
	}
	prepared, err := masterRecordedNarration(context.Background(), dataDir, "project_001", "run_001", resolved, runner)
	if err != nil {
		t.Fatalf("masterRecordedNarration() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
	joined := strings.Join(calls[0], " ")
	for _, token := range []string{resolved.ContentPath, "loudnorm=I=-16:TP=-1.5:LRA=7", "-ar 48000", "-ac 1", "pcm_s16le"} {
		if !strings.Contains(joined, token) {
			t.Fatalf("master command missing %q: %s", token, joined)
		}
	}
	if prepared.AudioPath == resolved.ContentPath || !strings.HasSuffix(prepared.AudioPath, "narration_master.wav") {
		t.Fatalf("AudioPath = %q", prepared.AudioPath)
	}
	provenanceBytes, err := os.ReadFile(prepared.ProvenancePath)
	if err != nil {
		t.Fatal(err)
	}
	var provenance map[string]interface{}
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance["schemaVersion"] != "tangying-production-audio-provenance/v1" || provenance["sourceMode"] != "recorded_narration" || provenance["productionReady"] != true || provenance["usageRightsConfirmed"] != true {
		t.Fatalf("provenance = %#v", provenance)
	}
	if provenance["inputFileSha256"] != strings.TrimPrefix(hash, "sha256:") || provenance["outputFileSha256"] == "" {
		t.Fatalf("provenance hashes = %#v", provenance)
	}
}
