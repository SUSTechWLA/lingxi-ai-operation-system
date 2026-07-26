package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestRevisionServiceRestoreCreatesImmutableCurrentChild(t *testing.T) {
	ctx := context.Background()
	repo := newRevisionServiceFake(t,
		&Artifact{
			ID: "artifact-v1", ProjectID: "project-1", StageName: "script", UnitID: "main",
			Kind: KindMarkdown, Name: "script.md", Version: 1, StorageType: StorageLocal,
			StorageRef: "local://original", MimeType: "text/markdown", SizeBytes: 12,
			ContentHash: "hash-v1", Metadata: map[string]interface{}{"nested": map[string]interface{}{"kept": true}},
		},
		&Artifact{
			ID: "artifact-v3", ProjectID: "project-1", StageName: "script", UnitID: "main",
			Kind: KindMarkdown, Name: "script.md", Version: 3, StorageType: StorageLocal,
			StorageRef: "local://current", MimeType: "text/markdown", SizeBytes: 12,
			ContentHash: "hash-v3", IsCurrent: true, Metadata: map[string]interface{}{"current": true},
		},
	)
	originalV1 := cloneArtifactForRevisionTest(repo.byID["artifact-v1"])
	revisions := NewRevisionService(repo)

	result, err := revisions.Restore(ctx, RestoreRequest{
		ArtifactID: "artifact-v1", ReviewerID: "user-1", Reason: "恢复第一版",
	})
	if err != nil {
		t.Fatalf("Restore error: %v", err)
	}
	if result.Artifact.Version != 4 || result.Artifact.ParentID != "artifact-v3" {
		t.Fatalf("restored artifact = %+v", result.Artifact)
	}
	if result.Artifact.Metadata["restoredFromArtifactId"] != "artifact-v1" {
		t.Fatalf("metadata = %+v", result.Artifact.Metadata)
	}
	if result.Artifact.Metadata["restoredByReviewerId"] != "user-1" || result.Artifact.Metadata["restoreReason"] != "恢复第一版" {
		t.Fatalf("restore attribution = %+v", result.Artifact.Metadata)
	}
	if result.Artifact.StorageRef != originalV1.StorageRef || result.Artifact.ContentHash != originalV1.ContentHash {
		t.Fatalf("restore did not preserve stored content identity: %+v", result.Artifact)
	}
	if !reflect.DeepEqual(repo.byID["artifact-v1"], originalV1) {
		t.Fatal("historical artifact was mutated")
	}
}

func TestRevisionServiceDirectContentBypassesGeneratorAndMarksDownstreamOnce(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	called := false
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		called = true
		return "generated", nil
	})

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "use supplied text", DirectContent: []byte("direct replacement"),
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if called {
		t.Fatal("direct content must not call the revision generator")
	}
	if string(repo.lastCreate.Data) != "direct replacement" || result.Artifact.Version != 2 {
		t.Fatalf("direct revision = %+v, create request = %+v", result.Artifact, repo.lastCreate)
	}
	if repo.stale != 1 || !reflect.DeepEqual(result.StaleStageNames, []string{"downstream"}) {
		t.Fatalf("downstream stale calls = %d, stages = %v", repo.stale, result.StaleStageNames)
	}
}

func TestRevisionServiceCopiesStructuredCreatorProvenanceIntoNewArtifact(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	selection := map[string]interface{}{"kind": "time", "startMs": int64(100), "endMs": int64(900)}

	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", DirectContent: []byte("direct replacement"),
		Provenance: map[string]interface{}{"mode": "direct", "baseVersion": 1, "selection": selection},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	want := map[string]interface{}{"kind": "time", "startMs": int64(100), "endMs": int64(900)}
	if got := repo.lastCreate.Metadata["selection"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection metadata = %#v, want %#v", got, want)
	}
	selection["startMs"] = int64(500)
	if got := repo.lastCreate.Metadata["selection"].(map[string]interface{})["startMs"]; got != int64(100) {
		t.Fatalf("stored provenance aliased caller selection: %#v", repo.lastCreate.Metadata["selection"])
	}
}

func TestRevisionServiceDirectIdenticalContentForcesNewVersion(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "keep content", DirectContent: []byte("original content"),
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if !repo.lastCreate.ForceNewVersion || result.Artifact.ID == "artifact-v1" || result.Artifact.Version != 2 || result.Artifact.ParentID != "artifact-v1" {
		t.Fatalf("identical direct revision did not create a new current child: request=%+v artifact=%+v", repo.lastCreate, result.Artifact)
	}
	if repo.stale != 1 {
		t.Fatalf("identical direct revision stale calls = %d, want 1", repo.stale)
	}
}

func TestRevisionServiceInstructionUsesGeneratorAndForwardsTextProvider(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	provider := map[string]interface{}{"provider": "openai", "model": "gpt-test"}
	var systemPrompt, userPrompt string
	var gotOptions ReviseLLMOptions
	revisions.SetConfig("", func(_ context.Context, system, user string, options ReviseLLMOptions) (string, error) {
		systemPrompt, userPrompt, gotOptions = system, user, options
		return "generator replacement", nil
	})

	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "make it shorter",
		ModelProviders: map[string]interface{}{"text_to_text": provider},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if gotOptions.ModelProvider["model"] != "gpt-test" || !strings.Contains(systemPrompt, "script") ||
		!strings.Contains(userPrompt, "original content") || !strings.Contains(userPrompt, "make it shorter") {
		t.Fatalf("generator inputs = system=%q user=%q opts=%+v", systemPrompt, userPrompt, gotOptions)
	}
	if string(repo.lastCreate.Data) != "generator replacement" {
		t.Fatalf("generated content was not versioned: %q", repo.lastCreate.Data)
	}
}

func TestRevisionServiceSelectedInstructionStoresOnlyScopedReplacement(t *testing.T) {
	base := revisionTestArtifact()
	base.InlineJSON = "开头正文结尾"
	repo := newRevisionServiceFake(t, base)
	revisions := NewRevisionService(repo)
	var systemPrompt, userPrompt string
	revisions.SetConfig("", func(_ context.Context, system, user string, _ ReviseLLMOptions) (string, error) {
		systemPrompt, userPrompt = system, user
		return "新文", nil
	})

	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: base.ID,
		Message:    "改得更简洁",
		Provenance: map[string]interface{}{
			"selection": map[string]interface{}{"kind": "text", "start": 2, "end": 4, "text": "正文"},
		},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if got := string(repo.lastCreate.Data); got != "开头新文结尾" {
		t.Fatalf("scoped revision data = %q, want %q", got, "开头新文结尾")
	}
	if strings.Contains(systemPrompt, base.InlineJSON) || strings.Contains(systemPrompt, "正文") {
		t.Fatalf("stable system prompt contains request-specific source or selection: %q", systemPrompt)
	}
	wantSystemPrompt := `你是一个专业的内容返工助手，正在帮助用户修改「script」阶段的产物。

重要规则：
- 严格根据用户的修改意见，在原始内容的基础上进行修改
- 保持原始内容的整体结构和格式风格
- 只修改用户明确要求修改的部分，不要擅自改动其他内容
- 如果原始内容是 Markdown 格式，输出 Markdown
- 如果原始内容是 JSON 格式，输出严格符合相同结构的 JSON
- 不要引入原始内容中没有的新字段、新章节或额外内容
- 输出完整内容，不要省略或截断`
	if systemPrompt != wantSystemPrompt {
		t.Fatalf("selected revision changed the pre-task system prompt:\n got %q\nwant %q", systemPrompt, wantSystemPrompt)
	}
	if !strings.Contains(userPrompt, "只返回替换文字") || strings.Contains(userPrompt, "重新生成完整内容") {
		t.Fatalf("selected revision user prompt did not request replacement-only output: %q", userPrompt)
	}
}

func TestRevisionServiceDirectContentRejectsTextSelectionProvenance(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	result, err := NewRevisionService(repo).Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", DirectContent: []byte("full replacement"),
		Provenance: map[string]interface{}{
			"selection": map[string]interface{}{"kind": "text", "start": 0, "end": 8, "text": "original"},
		},
	})
	if !errors.Is(err, ErrRevisionInvalidReplacement) || result != nil || repo.lastCreate != nil {
		t.Fatalf("direct content with text selection must fail closed: result=%+v err=%v create=%+v", result, err, repo.lastCreate)
	}
}

func TestRevisionServiceSelectedInstructionNormalizesReplacementOnlyOutput(t *testing.T) {
	for _, test := range []struct {
		name, generated, want string
	}{
		{name: "fenced", generated: "```text\n新文\n```", want: "开头新文结尾"},
		{name: "standalone labeled", generated: "替换文字：\n新文", want: "开头新文结尾"},
		{name: "standalone explanation label", generated: "Explanation:\n新文", want: "开头新文结尾"},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := revisionTestArtifact()
			base.InlineJSON = "开头正文结尾"
			repo := newRevisionServiceFake(t, base)
			revisions := NewRevisionService(repo)
			revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) { return test.generated, nil })
			_, err := revisions.Revise(context.Background(), ReviseRequest{
				ArtifactID: base.ID, Message: "rewrite",
				Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 2, "end": 4, "text": "正文"}},
			})
			if err != nil || string(repo.lastCreate.Data) != test.want {
				t.Fatalf("normalized revision error=%v data=%q want=%q", err, repo.lastCreate.Data, test.want)
			}
		})
	}
}

func TestRevisionServiceSelectedInstructionPreservesLiteralPrefixes(t *testing.T) {
	for _, generated := range []string{
		"替换文字：这是正文",
		"Replacement: literal content",
		"说明：这是正文",
		"Explanation: literal content",
	} {
		t.Run(generated, func(t *testing.T) {
			base := revisionTestArtifact()
			base.InlineJSON = "开头正文结尾"
			repo := newRevisionServiceFake(t, base)
			revisions := NewRevisionService(repo)
			revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) { return generated, nil })
			_, err := revisions.Revise(context.Background(), ReviseRequest{
				ArtifactID: base.ID, Message: "rewrite",
				Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 2, "end": 4, "text": "正文"}},
			})
			var data []byte
			if repo.lastCreate != nil {
				data = repo.lastCreate.Data
			}
			if err != nil || string(data) != "开头"+generated+"结尾" {
				t.Fatalf("literal prefix error=%v data=%q", err, data)
			}
		})
	}
}

func TestRevisionServiceSelectedInstructionRejectsInlineExplanatoryPhrases(t *testing.T) {
	for _, generated := range []string{
		"修改后的文字如下：新文",
		"Here is the revised text: new text",
	} {
		t.Run(generated, func(t *testing.T) {
			base := revisionTestArtifact()
			base.InlineJSON = "开头正文结尾"
			repo := newRevisionServiceFake(t, base)
			revisions := NewRevisionService(repo)
			revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) { return generated, nil })
			result, err := revisions.Revise(context.Background(), ReviseRequest{
				ArtifactID: base.ID, Message: "rewrite",
				Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 2, "end": 4, "text": "正文"}},
			})
			if !errors.Is(err, ErrRevisionInvalidReplacement) || result != nil || repo.lastCreate != nil {
				t.Fatalf("inline explanation must fail closed: result=%+v err=%v create=%+v", result, err, repo.lastCreate)
			}
		})
	}
}

func TestRevisionServiceSelectedInstructionEscapesJSONSensitiveReplacement(t *testing.T) {
	base := revisionTestArtifact()
	base.Kind = KindJSON
	base.MimeType = "application/json"
	base.InlineJSON = `{"script":"开头正文结尾"}`
	repo := newRevisionServiceFake(t, base)
	revisions := NewRevisionService(repo)
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "新\"文\\下一行\n结束", nil
	})
	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: base.ID, Message: "rewrite",
		Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 13, "end": 15, "text": "正文"}},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(repo.lastCreate.Data, &decoded); err != nil {
		t.Fatalf("spliced JSON is invalid: %v data=%q", err, repo.lastCreate.Data)
	}
	if got, want := decoded["script"], "开头新\"文\\下一行\n结束结尾"; got != want {
		t.Fatalf("decoded script=%q want=%q", got, want)
	}
}

func TestRevisionServiceSelectedInstructionRejectsInvalidJSONFragment(t *testing.T) {
	base := revisionTestArtifact()
	base.Kind = KindJSON
	base.MimeType = "application/json"
	base.InlineJSON = `{"count":12}`
	repo := newRevisionServiceFake(t, base)
	revisions := NewRevisionService(repo)
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "not-json", nil
	})
	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: base.ID, Message: "rewrite",
		Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 9, "end": 11, "text": "12"}},
	})
	if !errors.Is(err, ErrRevisionInvalidReplacement) || result != nil || repo.lastCreate != nil {
		t.Fatalf("invalid JSON fragment must fail closed: result=%+v err=%v create=%+v", result, err, repo.lastCreate)
	}
}

func TestRevisionServiceSelectedInstructionPreservesNestedJSONEnvelope(t *testing.T) {
	base := revisionTestArtifact()
	base.Kind = KindJSON
	base.MimeType = "application/json"
	base.InlineJSON = `{"content":"{\"script\":\"开头\\n正文结尾\"}"}`
	repo := newRevisionServiceFake(t, base)
	revisions := NewRevisionService(repo)
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "\n新\"文", nil
	})
	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: base.ID, Message: "rewrite",
		Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 28, "end": 33, "text": `\\n正文`}},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	var outer map[string]string
	if err := json.Unmarshal(repo.lastCreate.Data, &outer); err != nil {
		t.Fatalf("outer JSON invalid: %v data=%q", err, repo.lastCreate.Data)
	}
	var inner map[string]string
	if err := json.Unmarshal([]byte(outer["content"]), &inner); err != nil {
		t.Fatalf("nested JSON invalid: %v content=%q", err, outer["content"])
	}
	if got, want := inner["script"], "开头\n新\"文结尾"; got != want {
		t.Fatalf("nested script=%q want=%q", got, want)
	}
}

func TestRevisionServiceSelectedInstructionRejectsExplanatoryOutput(t *testing.T) {
	base := revisionTestArtifact()
	base.InlineJSON = "开头正文结尾"
	repo := newRevisionServiceFake(t, base)
	revisions := NewRevisionService(repo)
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "以下是修改后的文字：新文", nil
	})
	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: base.ID, Message: "rewrite",
		Provenance: map[string]interface{}{"selection": map[string]interface{}{"kind": "text", "start": 2, "end": 4, "text": "正文"}},
	})
	if !errors.Is(err, ErrRevisionInvalidReplacement) || result != nil || repo.lastCreate != nil {
		t.Fatalf("explanatory output must fail closed: result=%+v err=%v create=%+v", result, err, repo.lastCreate)
	}
}

func TestBuildSelectedRevisionUserPromptClampsEmojiContextBoundaries(t *testing.T) {
	source := "🙂" + strings.Repeat("前", 319) + "正文" + strings.Repeat("后", 319) + "🙂"
	selection := revisionTextSelection{Start: 321, End: 323, Text: "正文"}
	prompt, err := buildSelectedRevisionUserPrompt(source, selection, "rewrite")
	if err != nil || strings.Contains(prompt, "�") {
		t.Fatalf("emoji context boundary prompt error=%v prompt=%q", err, prompt)
	}
}

func TestRevisionServiceSelectedInstructionKeepsStablePromptAndBoundsContext(t *testing.T) {
	source := strings.Repeat("前", 400) + "正文" + strings.Repeat("后", 400)
	selection := map[string]interface{}{"kind": "text", "start": 400, "end": 402, "text": "正文"}
	var systemPrompts, userPrompts []string
	for index, message := range []string{"第一次修改", "第二次修改"} {
		base := revisionTestArtifact()
		base.ID = fmt.Sprintf("artifact-v%d", index+1)
		base.InlineJSON = source
		repo := newRevisionServiceFake(t, base)
		revisions := NewRevisionService(repo)
		revisions.SetConfig("", func(_ context.Context, system, user string, _ ReviseLLMOptions) (string, error) {
			systemPrompts = append(systemPrompts, system)
			userPrompts = append(userPrompts, user)
			return "新文", nil
		})
		if _, err := revisions.Revise(context.Background(), ReviseRequest{
			ArtifactID: base.ID, Message: message, Provenance: map[string]interface{}{"selection": selection},
		}); err != nil {
			t.Fatalf("Revise %d error: %v", index+1, err)
		}
	}
	if len(systemPrompts) != 2 || systemPrompts[0] != systemPrompts[1] {
		t.Fatalf("selected revisions changed stable system prompt: %#v", systemPrompts)
	}
	if len(userPrompts) != 2 || userPrompts[0] == userPrompts[1] {
		t.Fatalf("selected revisions must vary only in user delta: %#v", userPrompts)
	}
	if strings.Contains(userPrompts[0], strings.Repeat("前", 321)) || strings.Contains(userPrompts[0], strings.Repeat("后", 321)) {
		t.Fatalf("selected revision context exceeded 320 UTF-16 units: %q", userPrompts[0])
	}
}

func TestRevisionServiceMalformedTextSelectionDoesNotFallBackToFullRevision(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	called := false
	revisions.SetConfig("", func(_ context.Context, _, _ string, _ ReviseLLMOptions) (string, error) {
		called = true
		return "replacement", nil
	})

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "rewrite",
		Provenance: map[string]interface{}{
			"selection": map[string]interface{}{"kind": "text", "start": "1", "end": 3, "text": "ri"},
		},
	})
	if !errors.Is(err, ErrRevisionInvalidReplacement) || result != nil || called || repo.lastCreate != nil {
		t.Fatalf("malformed selection must fail closed: result=%+v err=%v called=%v create=%+v", result, err, called, repo.lastCreate)
	}
}

func TestRevisionServiceSelectedInstructionRequiresScopedGenerator(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	result, err := NewRevisionService(repo).Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "rewrite",
		Provenance: map[string]interface{}{
			"selection": map[string]interface{}{"kind": "text", "start": 0, "end": 8, "text": "original"},
		},
	})
	if !errors.Is(err, ErrRevisionGeneration) || result != nil || repo.lastCreate != nil {
		t.Fatalf("selected instruction without generator must not create a full local revision: result=%+v err=%v create=%+v", result, err, repo.lastCreate)
	}
}

func TestRevisionServiceIdenticalGeneratedContentForcesNewVersion(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "original content", nil
	})

	result, err := revisions.Revise(context.Background(), ReviseRequest{ArtifactID: "artifact-v1", Message: "keep content"})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if !repo.lastCreate.ForceNewVersion || result.Artifact.ID == "artifact-v1" || result.Artifact.Version != 2 || result.Artifact.ParentID != "artifact-v1" {
		t.Fatalf("identical generated revision did not create a new current child: request=%+v artifact=%+v", repo.lastCreate, result.Artifact)
	}
	if repo.stale != 1 {
		t.Fatalf("identical generated revision stale calls = %d, want 1", repo.stale)
	}
}

func TestRevisionServiceFailedCreateDoesNotMarkDownstreamStale(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	repo.createErr = errRevisionTestCreate
	revisions := NewRevisionService(repo)

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "replace", DirectContent: []byte("replacement"),
	})
	if err != errRevisionTestCreate || result != nil {
		t.Fatalf("failed revision = result=%+v err=%v", result, err)
	}
	if repo.stale != 0 {
		t.Fatalf("failed revision stale calls = %d, want 0", repo.stale)
	}
}

func TestRevisionServiceReplaceImageUsesCanonicalLocalIdentityWithoutGenerator(t *testing.T) {
	base := revisionTestArtifact()
	base.Kind = KindImage
	base.Name = "shot-02.png"
	base.MimeType = "image/png"
	base.StorageType = StorageLocal
	base.StorageRef = "local://projects/project-1/artifacts/shot-02/original.png"
	base.SizeBytes = 12
	base.ContentHash = "sha256:original"
	base.WorkflowRunID = "run-current"
	base.TaskID = "task-current"
	base.RoleAgentID = "role-current"
	base.ProducedByNode = "node-current"
	base.ProducedByTool = "tool-current"
	base.ProducedByRole = "role-current"
	base.Metadata = map[string]interface{}{
		"producedByNode": "node-current",
		"producedByTool": "tool-current",
		"producedByRole": "role-current",
		"nested":         map[string]interface{}{"kept": true},
		"localPath":      "/stale/original.png",
		"storageRef":     base.StorageRef,
		"contentHash":    base.ContentHash,
		"mimeType":       base.MimeType,
		"sizeBytes":      base.SizeBytes,
	}
	repo := newRevisionServiceFake(t, base)
	revisions := NewRevisionService(repo)
	generatorCalled := false
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		generatorCalled = true
		return "must not run", nil
	})
	replacement := ReplacementMaterialIdentity{
		ContentHash: "sha256:replacement",
		StorageRef:  "local://projects/project-1/materials/replacement",
		MimeType:    "image/webp",
		SizeBytes:   4096,
	}
	selection := map[string]interface{}{"kind": "rect", "x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4}

	result, err := revisions.Replace(context.Background(), ReplaceRequest{
		ArtifactID: "artifact-v1", BaseVersion: base.Version, NewArtifactID: "artifact-replacement",
		Material: replacement,
		Provenance: map[string]interface{}{
			"mode": "replace", "replacementMaterial": replacement, "selection": selection,
		},
	})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if generatorCalled {
		t.Fatal("typed image replacement must never invoke the revision generator")
	}
	if result.Artifact.Version != 2 || result.Artifact.ParentID != base.ID || result.Artifact.Kind != KindImage {
		t.Fatalf("replacement artifact = %+v", result.Artifact)
	}
	got := repo.lastCreate
	if got.ID != "artifact-replacement" || got.ProjectID != base.ProjectID || got.StageName != base.StageName ||
		got.UnitID != base.UnitID || got.Name != base.Name || got.Kind != KindImage || !got.ForceNewVersion {
		t.Fatalf("replacement request lost immutable identity: %+v", got)
	}
	if got.ExpectedParentID != base.ID || got.ExpectedParentVersion != base.Version {
		t.Fatalf("replacement request lost authorized base lineage: %+v", got)
	}
	if got.StorageType != StorageLocal || got.StorageRef != replacement.StorageRef || got.MimeType != replacement.MimeType ||
		got.SizeBytes != replacement.SizeBytes || got.ContentHash != replacement.ContentHash || len(got.Data) != 0 {
		t.Fatalf("replacement request did not use canonical local identity: %+v", got)
	}
	if got.WorkflowRunID != "run-current" || got.TaskID != "task-current" || got.RoleAgentID != "role-current" ||
		got.Metadata["producedByNode"] != "node-current" || got.Metadata["producedByTool"] != "tool-current" ||
		got.Metadata["producedByRole"] != "role-current" {
		t.Fatalf("replacement request lost current producer identity: %+v metadata=%+v", got, got.Metadata)
	}
	if !reflect.DeepEqual(got.Metadata["replacementMaterial"], replacement) ||
		!reflect.DeepEqual(got.Metadata["selection"], selection) {
		t.Fatalf("replacement provenance = %+v", got.Metadata)
	}
	for _, staleIdentity := range []string{"localPath", "storageRef", "contentHash", "mimeType", "sizeBytes", "mediaUrl", "mediaUrls"} {
		if _, exists := got.Metadata[staleIdentity]; exists {
			t.Fatalf("replacement resurrected stale %s metadata: %+v", staleIdentity, got.Metadata)
		}
	}
	if repo.stale != 1 {
		t.Fatalf("replacement stale calls = %d, want 1", repo.stale)
	}
}

func TestRevisionServiceReplaceRejectsInvalidTargetOrMaterialBeforeCreate(t *testing.T) {
	valid := ReplacementMaterialIdentity{
		ContentHash: "sha256:replacement",
		StorageRef:  "local://projects/project-1/materials/replacement",
		MimeType:    "image/png",
		SizeBytes:   1,
	}
	tests := []struct {
		name     string
		kind     ArtifactKind
		material ReplacementMaterialIdentity
	}{
		{name: "non image target", kind: KindMarkdown, material: valid},
		{name: "non local ref", kind: KindImage, material: ReplacementMaterialIdentity{ContentHash: valid.ContentHash, StorageRef: "https://example.test/replacement.png", MimeType: valid.MimeType, SizeBytes: valid.SizeBytes}},
		{name: "empty local ref", kind: KindImage, material: ReplacementMaterialIdentity{ContentHash: valid.ContentHash, StorageRef: "local://", MimeType: valid.MimeType, SizeBytes: valid.SizeBytes}},
		{name: "non image mime", kind: KindImage, material: ReplacementMaterialIdentity{ContentHash: valid.ContentHash, StorageRef: valid.StorageRef, MimeType: "video/mp4", SizeBytes: valid.SizeBytes}},
		{name: "empty image subtype", kind: KindImage, material: ReplacementMaterialIdentity{ContentHash: valid.ContentHash, StorageRef: valid.StorageRef, MimeType: "image/", SizeBytes: valid.SizeBytes}},
		{name: "negative size", kind: KindImage, material: ReplacementMaterialIdentity{ContentHash: valid.ContentHash, StorageRef: valid.StorageRef, MimeType: valid.MimeType, SizeBytes: -1}},
		{name: "invalid hash", kind: KindImage, material: ReplacementMaterialIdentity{ContentHash: "md5:bad", StorageRef: valid.StorageRef, MimeType: valid.MimeType, SizeBytes: valid.SizeBytes}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := revisionTestArtifact()
			base.Kind = test.kind
			repo := newRevisionServiceFake(t, base)
			result, err := NewRevisionService(repo).Replace(context.Background(), ReplaceRequest{
				ArtifactID:  base.ID,
				BaseVersion: base.Version,
				Material:    test.material,
			})
			if !errors.Is(err, ErrRevisionInvalidReplacement) || result != nil {
				t.Fatalf("Replace() result=%+v error=%v", result, err)
			}
			if repo.lastCreate != nil || repo.stale != 0 || !base.IsCurrent {
				t.Fatalf("invalid replacement mutated state: create=%+v stale=%d current=%v", repo.lastCreate, repo.stale, base.IsCurrent)
			}
		})
	}
}

func TestRevisionServiceReplaceCreateFailureLeavesCurrentImageAndInvalidationUntouched(t *testing.T) {
	base := revisionTestArtifact()
	base.Kind = KindImage
	repo := newRevisionServiceFake(t, base)
	repo.createErr = errRevisionTestCreate
	result, err := NewRevisionService(repo).Replace(context.Background(), ReplaceRequest{
		ArtifactID:  base.ID,
		BaseVersion: base.Version,
		Material: ReplacementMaterialIdentity{
			ContentHash: "sha256:replacement",
			StorageRef:  "local://projects/project-1/materials/replacement",
			MimeType:    "image/png",
			SizeBytes:   5,
		},
	})
	if err != errRevisionTestCreate || result != nil {
		t.Fatalf("Replace() result=%+v error=%v", result, err)
	}
	if repo.stale != 0 || !base.IsCurrent {
		t.Fatalf("failed replacement mutated current image: stale=%d current=%v", repo.stale, base.IsCurrent)
	}
}

func TestRevisionServiceReplaceConflictAfterBaseLoadKeepsCompetingCurrent(t *testing.T) {
	base := revisionTestArtifact()
	base.Kind = KindImage
	base.MimeType = "image/png"
	base.StorageRef = "local://projects/project-1/artifacts/original"
	base.ContentHash = "sha256:original"
	repo := newRevisionServiceFake(t, base)
	competing := cloneArtifactForRevisionTest(base)
	competing.ID = "artifact-v2"
	competing.Version = 2
	competing.ParentID = base.ID
	competing.ContentHash = "sha256:competing"
	competing.StorageRef = "local://projects/project-1/artifacts/competing"
	repo.beforeCreate = func(_ *CreateArtifactRequest) {
		base.IsCurrent = false
		competing.IsCurrent = true
		repo.byID[competing.ID] = competing
	}

	result, err := NewRevisionService(repo).Replace(context.Background(), ReplaceRequest{
		ArtifactID:    base.ID,
		BaseVersion:   base.Version,
		NewArtifactID: "artifact-replacement",
		Material: ReplacementMaterialIdentity{
			ContentHash: "sha256:replacement",
			StorageRef:  "local://projects/project-1/materials/replacement",
			MimeType:    "image/webp",
			SizeBytes:   42,
		},
	})
	if !errors.Is(err, ErrArtifactVersionConflict) || result != nil {
		t.Fatalf("Replace() result=%+v error=%v, want atomic version conflict", result, err)
	}
	current, currentErr := repo.GetCurrent(context.Background(), base.ProjectID, base.StageName, base.UnitID)
	if currentErr != nil || current.ID != competing.ID || !competing.IsCurrent {
		t.Fatalf("current artifact after conflict = %+v error=%v, want competing revision", current, currentErr)
	}
	if repo.stale != 0 || len(repo.byID) != 2 {
		t.Fatalf("conflict mutated replacement lineage: stale=%d artifacts=%d", repo.stale, len(repo.byID))
	}
}

func TestForcedRestoreVersionBypassesOnlyContentHashDedupAndPreservesStoredIdentity(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindMarkdown, Name: "script.md",
		StorageType: StorageLocal, StorageRef: "local://historical", ContentHash: "unchanged-hash",
		Metadata:        map[string]interface{}{"nested": map[string]interface{}{"immutable": true}},
		ForceNewVersion: true, RestoredFromID: "artifact-v1",
	}
	if contentHashDedupEnabled(req) {
		t.Fatal("forced restores must bypass content-hash deduplication")
	}
	if !contentHashDedupEnabled(&CreateArtifactRequest{ContentHash: "unchanged-hash"}) {
		t.Fatal("ordinary artifact creation must retain content-hash deduplication")
	}
	record := buildArtifactRecord(req, 4, "artifact-v3")
	if record.StorageRef != req.StorageRef || record.ContentHash != req.ContentHash || record.Kind != req.Kind {
		t.Fatalf("restore identity was changed: %+v", record)
	}
	if record.Metadata["restoredFromArtifactId"] != "artifact-v1" {
		t.Fatalf("restore provenance missing: %+v", record.Metadata)
	}
	record.Metadata["nested"].(map[string]interface{})["mutated"] = true
	if req.Metadata["nested"].(map[string]interface{})["mutated"] != nil {
		t.Fatal("restored metadata aliases the historical metadata map")
	}
	inlineRecord := buildArtifactRecord(&CreateArtifactRequest{
		ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindMarkdown, Name: "script.md",
		StorageType: StorageInline, StorageRef: "local://inline", Data: []byte("historical inline bytes"),
		ContentHash: "inline-hash", Provider: "legacy-provider", ForceNewVersion: true,
	}, 5, "artifact-v4")
	if inlineRecord.StorageType != StorageInline || inlineRecord.InlineJSON != "historical inline bytes" {
		t.Fatalf("forced restore did not preserve inline bytes: %+v", inlineRecord)
	}
}

func TestRevisionServiceRestorePreservesRemoteStorageTypes(t *testing.T) {
	for _, storageType := range []string{StorageMinIO, "url"} {
		t.Run(storageType, func(t *testing.T) {
			historical := revisionTestArtifact()
			historical.StorageType = storageType
			historical.StorageRef = "https://media.example.test/restored.mp4"
			historical.ContentHash = "historical-hash"
			historical.InlineJSON = ""
			historical.IsCurrent = false
			current := cloneArtifactForRevisionTest(historical)
			current.ID, current.Version, current.IsCurrent = "artifact-v3", 3, true
			repo := newRevisionServiceFake(t, historical, current)

			result, err := NewRevisionService(repo).Restore(context.Background(), RestoreRequest{ArtifactID: historical.ID})
			if err != nil {
				t.Fatalf("Restore error: %v", err)
			}
			if result.Artifact.StorageType != storageType || result.Artifact.StorageRef != historical.StorageRef || result.Artifact.ContentHash != historical.ContentHash {
				t.Fatalf("remote restore identity = %+v", result.Artifact)
			}
			if len(repo.lastCreate.Data) != 0 {
				t.Fatalf("remote restore must retain external bytes by reference, got %q", repo.lastCreate.Data)
			}
		})
	}
}

func TestForcedRestoreRecordPreservesRemoteMediaURLCompatibility(t *testing.T) {
	for _, storageType := range []string{StorageMinIO, "url"} {
		t.Run(storageType, func(t *testing.T) {
			ref := "https://media.example.test/restored.mp4"
			restored := buildArtifactRecord(&CreateArtifactRequest{
				ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindVideo, Name: "restored.mp4",
				StorageType: storageType, StorageRef: ref, ContentHash: "historical-hash", ForceNewVersion: true,
			}, 4, "artifact-v3")
			if restored.StorageType != storageType || restored.StorageRef != ref {
				t.Fatalf("forced record changed remote storage: %+v", restored)
			}
			_, mediaURL, mediaURLs := artifactContent(restored)
			if mediaURL != ref || !reflect.DeepEqual(mediaURLs, []string{ref}) {
				t.Fatalf("legacy remote media response = mediaURL=%q mediaURLs=%v", mediaURL, mediaURLs)
			}
		})
	}
}

func TestRestoreDeepClonesTypedMutableMetadata(t *testing.T) {
	historical := revisionTestArtifact()
	historical.IsCurrent = false
	historical.Metadata = revisionTypedMetadata()
	current := *historical
	current.Metadata = revisionTypedMetadata()
	current.ID, current.Version, current.IsCurrent = "artifact-v3", 3, true
	originalHistorical := revisionTypedMetadata()
	originalCurrent := revisionTypedMetadata()
	repo := newRevisionServiceFake(t, historical, &current)

	result, err := NewRevisionService(repo).Restore(context.Background(), RestoreRequest{ArtifactID: historical.ID})
	if err != nil {
		t.Fatalf("Restore error: %v", err)
	}
	metadata := result.Artifact.Metadata
	metadata["bytes"].([]byte)[0] = 9
	metadata["strings"].([]string)[0] = "changed"
	metadata["typedMap"].(map[string]string)["owner"] = "changed"
	metadata["typedSlice"].([]revisionMetadataValue)[0].Labels[0] = "changed"
	metadata["pointer"].(*revisionMetadataValue).Labels[0] = "changed"
	if !reflect.DeepEqual(historical.Metadata, originalHistorical) {
		t.Fatalf("historical metadata was aliased: %+v", historical.Metadata)
	}
	if !reflect.DeepEqual(current.Metadata, originalCurrent) {
		t.Fatalf("current metadata was aliased: %+v", current.Metadata)
	}
}

func TestCloneMetadataPreservesCyclicValuesWithoutAliasing(t *testing.T) {
	cycle := map[string]interface{}{}
	cycle["self"] = cycle
	cloned := cloneMetadata(map[string]interface{}{"cycle": cycle})
	clonedCycle := cloned["cycle"].(map[string]interface{})
	clonedCycle["self"].(map[string]interface{})["changedThroughCycle"] = true
	if cycle["changedThroughCycle"] != nil {
		t.Fatal("cyclic metadata clone aliases the original map")
	}
	if clonedCycle["changedThroughCycle"] != true {
		t.Fatal("cyclic metadata clone did not preserve the cycle")
	}
}

func TestReviseArtifactKeepsLegacySuccessResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	handler := (&Handler{revisions: NewRevisionService(repo)}).WithProjectAccess(ProjectAccessFunc(func(_ context.Context, userID, projectID string) bool {
		return userID == "user-1" && projectID == "project-1"
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/artifacts/artifact-v1/revise", strings.NewReader(`{"message":"make it tighter"}`))
	request = request.WithContext(auth.ContextWithUser(request.Context(), "user-1"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Params = gin.Params{{Key: "id", Value: "artifact-v1"}}

	handler.ReviseArtifact(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("legacy revise status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Code != 200 || response.Message != "success" {
		t.Fatalf("legacy response = %s, unmarshal error = %v", recorder.Body.String(), err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(response.Data, &data); err != nil || data["artifact"] == nil || data["content"] == nil || data["mediaUrls"] == nil {
		t.Fatalf("legacy data = %s, unmarshal error = %v", response.Data, err)
	}
}

func TestHandlerSetRevisionConfigConfiguresSharedService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	handler := (&Handler{revisions: NewRevisionService(repo)}).WithProjectAccess(ProjectAccessFunc(func(_ context.Context, userID, projectID string) bool {
		return userID == "user-1" && projectID == "project-1"
	}))
	handler.SetRevisionConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "configured generator content", nil
	})
	request := httptest.NewRequest(http.MethodPost, "/api/artifacts/artifact-v1/revise", strings.NewReader(`{"message":"make it tighter"}`))
	request = request.WithContext(auth.ContextWithUser(request.Context(), "user-1"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Params = gin.Params{{Key: "id", Value: "artifact-v1"}}

	handler.ReviseArtifact(context)

	if recorder.Code != http.StatusOK || string(repo.lastCreate.Data) != "configured generator content" {
		t.Fatalf("handler did not use configured shared generator: status=%d data=%q", recorder.Code, repo.lastCreate.Data)
	}
}

func revisionTestArtifact() *Artifact {
	return &Artifact{
		ID: "artifact-v1", ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindMarkdown,
		Name: "script.md", Version: 1, StorageType: StorageInline, InlineJSON: "original content",
		StorageRef: "local://original", MimeType: "text/markdown", ContentHash: "hash-v1", IsCurrent: true,
		Metadata: map[string]interface{}{},
	}
}

func TestRestoreUsesHistoricalBytesButCurrentExecutionIdentity(t *testing.T) {
	historical := revisionTestArtifact()
	historical.ID, historical.Version, historical.IsCurrent = "artifact-v1", 1, false
	historical.WorkflowRunID, historical.TaskID, historical.RoleAgentID = "old-run", "old-task", "old-role"
	historical.Metadata = map[string]interface{}{"producedByNode": "old-node", "producedByTool": "old-tool", "producedByRole": "old-role"}
	current := cloneArtifactForRevisionTest(historical)
	current.ID, current.Version, current.IsCurrent = "artifact-v3", 3, true
	current.WorkflowRunID, current.TaskID, current.RoleAgentID = "new-run", "new-task", "new-role"
	current.ProducedByNode, current.ProducedByTool, current.ProducedByRole = "new-node", "new-tool", "new-role"
	current.Metadata = map[string]interface{}{}
	repo := newRevisionServiceFake(t, historical, current)

	if _, err := NewRevisionService(repo).Restore(context.Background(), RestoreRequest{ArtifactID: historical.ID}); err != nil {
		t.Fatal(err)
	}
	got := repo.lastCreate
	if got.WorkflowRunID != "new-run" || got.TaskID != "new-task" || got.RoleAgentID != "new-role" {
		t.Fatalf("execution identity = run=%q task=%q role=%q", got.WorkflowRunID, got.TaskID, got.RoleAgentID)
	}
	if got.Metadata["producedByNode"] != "new-node" || got.Metadata["producedByTool"] != "new-tool" || got.ContentHash != historical.ContentHash {
		t.Fatalf("request=%+v metadata=%+v", got, got.Metadata)
	}
}

type revisionServiceFake struct {
	t            *testing.T
	byID         map[string]*Artifact
	stale        int
	lastCreate   *CreateArtifactRequest
	createErr    error
	beforeCreate func(*CreateArtifactRequest)
}

func newRevisionServiceFake(t *testing.T, artifacts ...*Artifact) *revisionServiceFake {
	t.Helper()
	fake := &revisionServiceFake{t: t, byID: make(map[string]*Artifact, len(artifacts))}
	for _, artifact := range artifacts {
		fake.byID[artifact.ID] = artifact
	}
	return fake
}

func (f *revisionServiceFake) GetByID(_ context.Context, id string) (*Artifact, error) {
	artifact, ok := f.byID[id]
	if !ok {
		return nil, errRevisionTestNotFound
	}
	return artifact, nil
}

func (f *revisionServiceFake) GetCurrent(_ context.Context, projectID, stageName, unitID string) (*Artifact, error) {
	for _, artifact := range f.byID {
		if artifact.ProjectID == projectID && artifact.StageName == stageName && artifact.UnitID == unitID && artifact.IsCurrent {
			return artifact, nil
		}
	}
	return nil, errRevisionTestNotFound
}

func (f *revisionServiceFake) CreateArtifact(_ context.Context, req *CreateArtifactRequest) (*Artifact, error) {
	f.lastCreate = cloneCreateRequestForRevisionTest(req)
	if f.beforeCreate != nil {
		f.beforeCreate(req)
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	var current *Artifact
	for _, artifact := range f.byID {
		if artifact.ProjectID == req.ProjectID && artifact.StageName == req.StageName && artifact.UnitID == req.UnitID && artifact.IsCurrent {
			current = artifact
		}
	}
	if req.ExpectedParentID != "" && (current == nil || current.ID != req.ExpectedParentID || current.Version != req.ExpectedParentVersion) {
		return nil, ErrArtifactVersionConflict
	}
	if current == nil {
		f.t.Fatal("CreateArtifact must use an existing current artifact")
	}
	current.IsCurrent = false
	created := &Artifact{
		ID: "artifact-v4", ProjectID: req.ProjectID, WorkflowRunID: req.WorkflowRunID, TaskID: req.TaskID,
		StageName: req.StageName, RoleAgentID: req.RoleAgentID, UnitID: req.UnitID, Kind: req.Kind, Name: req.Name,
		Version: current.Version + 1, ParentID: current.ID, StorageType: req.StorageType, StorageRef: req.StorageRef,
		MimeType: req.MimeType, SizeBytes: req.SizeBytes, ContentHash: req.ContentHash, PromptHash: req.PromptHash,
		Provider: req.Provider, Model: req.Model, IsCurrent: true, Metadata: cloneMetadata(req.Metadata),
	}
	if req.StorageType == StorageInline {
		created.InlineJSON = string(req.Data)
	}
	if req.RestoredFromID != "" {
		created.Metadata["restoredFromArtifactId"] = req.RestoredFromID
	}
	f.byID[created.ID] = created
	return created, nil
}

func cloneCreateRequestForRevisionTest(req *CreateArtifactRequest) *CreateArtifactRequest {
	cloned := *req
	cloned.Data = append([]byte(nil), req.Data...)
	cloned.Metadata = cloneMetadata(req.Metadata)
	return &cloned
}

func (f *revisionServiceFake) MarkDownstreamStale(_ context.Context, _, _, _ string) ([]string, error) {
	f.stale++
	return []string{"downstream"}, nil
}

var errRevisionTestNotFound = &revisionTestError{}
var errRevisionTestCreate = &revisionTestCreateError{}

type revisionTestError struct{}

func (*revisionTestError) Error() string { return "not found" }

type revisionTestCreateError struct{}

func (*revisionTestCreateError) Error() string { return "create failed" }

type revisionMetadataValue struct {
	Labels []string
}

func revisionTypedMetadata() map[string]interface{} {
	return map[string]interface{}{
		"bytes":    []byte{1, 2},
		"strings":  []string{"historical"},
		"typedMap": map[string]string{"owner": "historical"},
		"typedSlice": []revisionMetadataValue{{
			Labels: []string{"historical"},
		}},
		"pointer": &revisionMetadataValue{Labels: []string{"historical"}},
	}
}

func cloneArtifactForRevisionTest(artifact *Artifact) *Artifact {
	copy := *artifact
	copy.Metadata = deepCloneRevisionMetadata(artifact.Metadata)
	return &copy
}

func deepCloneRevisionMetadata(metadata map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		switch typed := value.(type) {
		case map[string]interface{}:
			cloned[key] = deepCloneRevisionMetadata(typed)
		default:
			cloned[key] = typed
		}
	}
	return cloned
}
