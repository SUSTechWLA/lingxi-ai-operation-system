package observability

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeErrorFingerprintIgnoresDynamicMessageAndCause(t *testing.T) {
	a := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("dial port 9001"), "mcp-client", "evt_a")
	b := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("dial port 9002"), "mcp-client", "evt_b")

	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("fingerprints differ: %q %q", a.Fingerprint, b.Fingerprint)
	}
	if len(a.Fingerprint) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(a.Fingerprint))
	}
}

func TestNormalizeErrorFingerprintIncludesCodeAndComponent(t *testing.T) {
	baseline := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("dial port 9001"), "mcp-client", "")
	differentCode := NormalizeError("MCP.TOOL.NOT_FOUND", errors.New("dial port 9001"), "mcp-client", "")
	differentComponent := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("dial port 9001"), "registry-client", "")

	if baseline.Fingerprint == differentCode.Fingerprint {
		t.Fatal("changing error code did not change fingerprint")
	}
	if baseline.Fingerprint == differentComponent.Fingerprint {
		t.Fatal("changing component did not change fingerprint")
	}
}

func TestNormalizeErrorUsesApprovedRegistryMetadata(t *testing.T) {
	got := NormalizeError(
		"MCP.CONNECTION.UNAVAILABLE",
		errors.New("authorization Bearer raw-secret"),
		"mcp-client",
		"evt_cause",
	)
	if got == nil {
		t.Fatal("NormalizeError() = nil")
	}

	if got.Code != "MCP.CONNECTION.UNAVAILABLE" ||
		got.Class != ErrorClassConnection ||
		!got.Retryable ||
		got.CausedByEventID != "evt_cause" ||
		got.UserMessageKey != "error.mcp.connection.unavailable" ||
		got.DeveloperDetail != "diagnostic.mcp.connection.unavailable" ||
		got.SuggestedActionKey != "action.mcp.retry.connection" {
		t.Fatalf("NormalizeError() = %+v", got)
	}
	if strings.Contains(strings.ToLower(got.DeveloperDetail), "raw-secret") || ContainsSecret(got) {
		t.Fatalf("NormalizeError() preserved raw error text: %+v", got)
	}
}

func TestNormalizeErrorSupportsEveryApprovedCode(t *testing.T) {
	tests := []struct {
		code       string
		class      ErrorClass
		retryable  bool
		messageKey string
		actionKey  string
	}{
		{"AUTH.SESSION.EXPIRED", ErrorClassAuthentication, false, "error.auth.session.expired", "action.auth.sign.in"},
		{"REQUEST.HANDLER.FAILED", ErrorClassExecution, false, "error.request.handler.failed", "action.request.retry"},
		{"WORKFLOW.STAGE.TIMEOUT", ErrorClassTimeout, true, "error.workflow.stage.timeout", "action.workflow.retry.stage"},
		{"WORKFLOW.RUN.EXECUTION_FAILED", ErrorClassExecution, true, "error.workflow.run.execution.failed", "action.workflow.retry.run"},
		{"WORKFLOW.STAGE.EXECUTION_FAILED", ErrorClassExecution, true, "error.workflow.stage.execution.failed", "action.workflow.retry.stage"},
		{"AGENT.PLAN.VALIDATION_FAILED", ErrorClassValidation, false, "error.agent.plan.validation.failed", "action.agent.review.plan"},
		{"AGENT.PLAN.GENERATION_FAILED", ErrorClassExecution, true, "error.agent.plan.generation.failed", "action.agent.retry.plan"},
		{"AGENT.PLAN.COMPILATION_FAILED", ErrorClassExecution, false, "error.agent.plan.compilation.failed", "action.agent.review.plan"},
		{"AGENT.DAG.SUBMISSION_FAILED", ErrorClassConnection, true, "error.agent.dag.submission.failed", "action.agent.retry.submission"},
		{"AGENT.RUNTIME.INTERNAL_FAILURE", ErrorClassExecution, false, "error.agent.runtime.internal.failure", "action.agent.retry.run"},
		{"LLM.PROVIDER.RATE_LIMITED", ErrorClassProvider, true, "error.llm.provider.rate.limited", "action.llm.retry.later"},
		{"LLM.RESPONSE.SCHEMA_INVALID", ErrorClassValidation, true, "error.llm.response.schema.invalid", "action.llm.retry"},
		{"LLM.TRANSPORT.UNAVAILABLE", ErrorClassConnection, true, "error.llm.transport.unavailable", "action.llm.retry"},
		{"LLM.CALL.TIMEOUT", ErrorClassTimeout, true, "error.llm.call.timeout", "action.llm.retry"},
		{"LLM.CALL.INTERNAL_FAILURE", ErrorClassExecution, false, "error.llm.call.internal.failure", "action.llm.retry"},
		{"MCP.CONNECTION.UNAVAILABLE", ErrorClassConnection, true, "error.mcp.connection.unavailable", "action.mcp.retry.connection"},
		{"MCP.TOOL.NOT_FOUND", ErrorClassNotFound, false, "error.mcp.tool.not.found", "action.mcp.refresh.registry"},
		{"TOOL.ARGUMENT.SCHEMA_INVALID", ErrorClassValidation, false, "error.tool.argument.schema.invalid", "action.tool.correct.arguments"},
		{"TOOL.EXECUTION.FAILED", ErrorClassExecution, true, "error.tool.execution.failed", "action.tool.retry"},
		{"LOCAL.JOB.EXECUTION_FAILED", ErrorClassExecution, true, "error.local.job.execution.failed", "action.local.job.retry"},
		{"ARTIFACT.FILE.MISSING", ErrorClassNotFound, false, "error.artifact.file.missing", "action.artifact.regenerate"},
		{"ARTIFACT.HASH.MISMATCH", ErrorClassIntegrity, false, "error.artifact.hash.mismatch", "action.artifact.verify.source"},
		{"RENDER.BLENDER.PROCESS_FAILED", ErrorClassRender, true, "error.render.blender.process.failed", "action.render.retry"},
		{"RENDER.FFMPEG.CODEC_UNSUPPORTED", ErrorClassRender, false, "error.render.ffmpeg.codec.unsupported", "action.render.select.codec"},
		{"MEDIA.AUDIO.DURATION_MISMATCH", ErrorClassMedia, false, "error.media.audio.duration.mismatch", "action.media.regenerate.audio"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := NormalizeError(tt.code, errors.New("dynamic raw detail"), "test-component", "")
			if got == nil {
				t.Fatalf("NormalizeError(%q) = nil", tt.code)
			}
			if got.Code != tt.code ||
				got.Class != tt.class ||
				got.Retryable != tt.retryable ||
				got.UserMessageKey != tt.messageKey ||
				got.SuggestedActionKey != tt.actionKey {
				t.Fatalf("NormalizeError(%q) = %+v", tt.code, got)
			}
		})
	}
}

func TestNormalizeErrorRejectsUnregisteredCode(t *testing.T) {
	if got := NormalizeError("UNKNOWN.ERROR.CODE", errors.New("boom"), "component", ""); got != nil {
		t.Fatalf("NormalizeError() = %+v, want nil", got)
	}
}

func TestNormalizeErrorDropsInvalidCausedByEventID(t *testing.T) {
	got := NormalizeError(
		"MCP.CONNECTION.UNAVAILABLE",
		errors.New("boom"),
		"mcp-client",
		"authorization.Bearer secret-value",
	)

	if got.CausedByEventID != "" {
		t.Fatalf("CausedByEventID = %q, want empty", got.CausedByEventID)
	}
	if strings.Contains(got.CausedByEventID, "secret-value") {
		t.Fatal("invalid causal value retained secret text")
	}
}

func TestNormalizeErrorDropsSensitiveAndOversizedCausedByEventIDs(t *testing.T) {
	sensitive := []string{
		"evt_authorization_value",
		"evt_Bearer_value",
		"evt_cookie_value",
		"evt_token_value",
		"evt_password_value",
		"evt_apiKey_value",
		"evt_api_key_value",
		"evt_secret_value",
		"evt_prompt_value",
		"evt_userInput_value",
		"evt_user_input_value",
		"evt_sk_live_value",
	}
	for _, causedBy := range sensitive {
		got := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("boom"), "mcp-client", causedBy)
		if got.CausedByEventID != "" {
			t.Errorf("NormalizeError() retained sensitive cause %q", causedBy)
		}
	}

	oversized := "evt_" + strings.Repeat("a", 125)
	got := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("boom"), "mcp-client", oversized)
	if got.CausedByEventID != "" {
		t.Fatalf("NormalizeError() retained %d-byte cause", len(oversized))
	}
}

func TestNormalizeErrorPreservesSafeCanonicalCausedByEventID(t *testing.T) {
	const causedBy = "evt_01j99zstagefailed"
	got := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("boom"), "mcp-client", causedBy)

	if got.CausedByEventID != causedBy {
		t.Fatalf("CausedByEventID = %q, want %q", got.CausedByEventID, causedBy)
	}
}
