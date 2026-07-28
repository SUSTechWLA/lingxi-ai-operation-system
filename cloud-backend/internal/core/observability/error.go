package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type ErrorClass string

const (
	ErrorClassAuthentication ErrorClass = "AUTHENTICATION"
	ErrorClassTimeout        ErrorClass = "TIMEOUT"
	ErrorClassValidation     ErrorClass = "VALIDATION"
	ErrorClassProvider       ErrorClass = "PROVIDER"
	ErrorClassConnection     ErrorClass = "CONNECTION"
	ErrorClassNotFound       ErrorClass = "NOT_FOUND"
	ErrorClassIntegrity      ErrorClass = "INTEGRITY"
	ErrorClassRender         ErrorClass = "RENDER"
	ErrorClassMedia          ErrorClass = "MEDIA"
	ErrorClassExecution      ErrorClass = "EXECUTION"
)

type EventError struct {
	Code               string     `json:"code"`
	Class              ErrorClass `json:"class"`
	Fingerprint        string     `json:"fingerprint"`
	Retryable          bool       `json:"retryable"`
	CausedByEventID    string     `json:"causedByEventId,omitempty"`
	UserMessageKey     string     `json:"userMessageKey"`
	DeveloperDetail    string     `json:"developerDetail,omitempty"`
	SuggestedActionKey string     `json:"suggestedActionKey"`
	EvidenceRefs       []string   `json:"evidenceRefs,omitempty"`
	ProtectedStackRef  string     `json:"protectedStackRef,omitempty"`
}

type errorDefinition struct {
	Class              ErrorClass
	Retryable          bool
	UserMessageKey     string
	SuggestedActionKey string
}

const maxCausalEventReferenceLength = 128

var sensitiveCausalEventMarkers = []string{
	"authorization",
	"bearer",
	"cookie",
	"token",
	"password",
	"apikey",
	"secret",
	"prompt",
	"userinput",
	"sklive",
}

var errorRegistry = map[string]errorDefinition{
	"AUTH.SESSION.EXPIRED": {
		Class:              ErrorClassAuthentication,
		Retryable:          false,
		UserMessageKey:     "error.auth.session.expired",
		SuggestedActionKey: "action.auth.sign.in",
	},
	"REQUEST.HANDLER.FAILED": {Class: ErrorClassExecution, Retryable: false, UserMessageKey: "error.request.handler.failed", SuggestedActionKey: "action.request.retry"},
	"WORKFLOW.STAGE.TIMEOUT": {
		Class:              ErrorClassTimeout,
		Retryable:          true,
		UserMessageKey:     "error.workflow.stage.timeout",
		SuggestedActionKey: "action.workflow.retry.stage",
	},
	"WORKFLOW.RUN.EXECUTION_FAILED":   {Class: ErrorClassExecution, Retryable: true, UserMessageKey: "error.workflow.run.execution.failed", SuggestedActionKey: "action.workflow.retry.run"},
	"WORKFLOW.STAGE.EXECUTION_FAILED": {Class: ErrorClassExecution, Retryable: true, UserMessageKey: "error.workflow.stage.execution.failed", SuggestedActionKey: "action.workflow.retry.stage"},
	"AGENT.PLAN.VALIDATION_FAILED": {
		Class:              ErrorClassValidation,
		Retryable:          false,
		UserMessageKey:     "error.agent.plan.validation.failed",
		SuggestedActionKey: "action.agent.review.plan",
	},
	"AGENT.PLAN.GENERATION_FAILED":   {Class: ErrorClassExecution, Retryable: true, UserMessageKey: "error.agent.plan.generation.failed", SuggestedActionKey: "action.agent.retry.plan"},
	"AGENT.PLAN.COMPILATION_FAILED":  {Class: ErrorClassExecution, Retryable: false, UserMessageKey: "error.agent.plan.compilation.failed", SuggestedActionKey: "action.agent.review.plan"},
	"AGENT.DAG.SUBMISSION_FAILED":    {Class: ErrorClassConnection, Retryable: true, UserMessageKey: "error.agent.dag.submission.failed", SuggestedActionKey: "action.agent.retry.submission"},
	"AGENT.RUN.TIMEOUT":              {Class: ErrorClassTimeout, Retryable: true, UserMessageKey: "error.agent.run.timeout", SuggestedActionKey: "action.agent.retry.run"},
	"AGENT.RUNTIME.INTERNAL_FAILURE": {Class: ErrorClassExecution, Retryable: false, UserMessageKey: "error.agent.runtime.internal.failure", SuggestedActionKey: "action.agent.retry.run"},
	"LLM.PROVIDER.RATE_LIMITED": {
		Class:              ErrorClassProvider,
		Retryable:          true,
		UserMessageKey:     "error.llm.provider.rate.limited",
		SuggestedActionKey: "action.llm.retry.later",
	},
	"LLM.RESPONSE.SCHEMA_INVALID": {
		Class:              ErrorClassValidation,
		Retryable:          true,
		UserMessageKey:     "error.llm.response.schema.invalid",
		SuggestedActionKey: "action.llm.retry",
	},
	"LLM.TRANSPORT.UNAVAILABLE": {Class: ErrorClassConnection, Retryable: true, UserMessageKey: "error.llm.transport.unavailable", SuggestedActionKey: "action.llm.retry"},
	"LLM.CALL.TIMEOUT":          {Class: ErrorClassTimeout, Retryable: true, UserMessageKey: "error.llm.call.timeout", SuggestedActionKey: "action.llm.retry"},
	"LLM.CALL.INTERNAL_FAILURE": {Class: ErrorClassExecution, Retryable: false, UserMessageKey: "error.llm.call.internal.failure", SuggestedActionKey: "action.llm.retry"},
	"MCP.CONNECTION.UNAVAILABLE": {
		Class:              ErrorClassConnection,
		Retryable:          true,
		UserMessageKey:     "error.mcp.connection.unavailable",
		SuggestedActionKey: "action.mcp.retry.connection",
	},
	"MCP.TOOL.NOT_FOUND": {
		Class:              ErrorClassNotFound,
		Retryable:          false,
		UserMessageKey:     "error.mcp.tool.not.found",
		SuggestedActionKey: "action.mcp.refresh.registry",
	},
	"TOOL.ARGUMENT.SCHEMA_INVALID": {
		Class:              ErrorClassValidation,
		Retryable:          false,
		UserMessageKey:     "error.tool.argument.schema.invalid",
		SuggestedActionKey: "action.tool.correct.arguments",
	},
	"TOOL.EXECUTION.FAILED":      {Class: ErrorClassExecution, Retryable: true, UserMessageKey: "error.tool.execution.failed", SuggestedActionKey: "action.tool.retry"},
	"LOCAL.JOB.EXECUTION_FAILED": {Class: ErrorClassExecution, Retryable: true, UserMessageKey: "error.local.job.execution.failed", SuggestedActionKey: "action.local.job.retry"},
	"ARTIFACT.FILE.MISSING": {
		Class:              ErrorClassNotFound,
		Retryable:          false,
		UserMessageKey:     "error.artifact.file.missing",
		SuggestedActionKey: "action.artifact.regenerate",
	},
	"ARTIFACT.HASH.MISMATCH": {
		Class:              ErrorClassIntegrity,
		Retryable:          false,
		UserMessageKey:     "error.artifact.hash.mismatch",
		SuggestedActionKey: "action.artifact.verify.source",
	},
	"RENDER.BLENDER.PROCESS_FAILED": {
		Class:              ErrorClassRender,
		Retryable:          true,
		UserMessageKey:     "error.render.blender.process.failed",
		SuggestedActionKey: "action.render.retry",
	},
	"RENDER.FFMPEG.CODEC_UNSUPPORTED": {
		Class:              ErrorClassRender,
		Retryable:          false,
		UserMessageKey:     "error.render.ffmpeg.codec.unsupported",
		SuggestedActionKey: "action.render.select.codec",
	},
	"MEDIA.AUDIO.DURATION_MISMATCH": {
		Class:              ErrorClassMedia,
		Retryable:          false,
		UserMessageKey:     "error.media.audio.duration.mismatch",
		SuggestedActionKey: "action.media.regenerate.audio",
	},
}

// NormalizeError returns nil for an unregistered code. Raw error text is never
// copied into the event or fingerprint.
func NormalizeError(code string, err error, component string, causedBy string) *EventError {
	definition, ok := errorRegistry[code]
	if !ok {
		return nil
	}

	// The error parameter is intentionally excluded. error.Error() is dynamic
	// and may contain credentials, user content, endpoints, or local paths.
	_ = err
	fingerprintInput := code + "\x00" + strings.TrimSpace(component)
	sum := sha256.Sum256([]byte(fingerprintInput))
	causedByEventID := ""
	if safeCausalEventReference(causedBy) {
		causedByEventID = causedBy
	}

	return &EventError{
		Code:               code,
		Class:              definition.Class,
		Fingerprint:        hex.EncodeToString(sum[:]),
		Retryable:          definition.Retryable,
		CausedByEventID:    causedByEventID,
		UserMessageKey:     definition.UserMessageKey,
		DeveloperDetail:    diagnosticKey(code),
		SuggestedActionKey: definition.SuggestedActionKey,
	}
}

func safeCausalEventReference(value string) bool {
	if len(value) > maxCausalEventReferenceLength || !eventIDPattern.MatchString(value) {
		return false
	}
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(value))
	for _, marker := range sensitiveCausalEventMarkers {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	return true
}

func diagnosticKey(code string) string {
	key := strings.ToLower(strings.ReplaceAll(code, "_", "."))
	return "diagnostic." + key
}

func matchesDiagnosticKey(code, detail string) bool {
	if detail == "" {
		return true
	}
	if _, ok := errorRegistry[code]; !ok {
		return false
	}
	return detail == diagnosticKey(code)
}

func (e EventError) validate() error {
	definition, ok := errorRegistry[e.Code]
	if !ok {
		return fmt.Errorf("error.code is not in the approved registry")
	}
	if e.Class != definition.Class {
		return fmt.Errorf("error.class does not match error.code")
	}
	if e.Retryable != definition.Retryable {
		return fmt.Errorf("error.retryable does not match error.code")
	}
	if e.UserMessageKey != definition.UserMessageKey {
		return fmt.Errorf("error.userMessageKey does not match error.code")
	}
	if e.SuggestedActionKey != definition.SuggestedActionKey {
		return fmt.Errorf("error.suggestedActionKey does not match error.code")
	}
	if !hashPattern.MatchString(e.Fingerprint) {
		return fmt.Errorf("error.fingerprint has an invalid format")
	}
	if e.CausedByEventID != "" && !safeCausalEventReference(e.CausedByEventID) {
		return fmt.Errorf("error.causedByEventId is not a safe causal reference")
	}
	if !matchesDiagnosticKey(e.Code, e.DeveloperDetail) {
		return fmt.Errorf("error.developerDetail does not match error.code")
	}
	if err := validateArtifactRefs("error.evidenceRefs", e.EvidenceRefs); err != nil {
		return err
	}
	if e.ProtectedStackRef != "" && !strings.HasPrefix(e.ProtectedStackRef, "artifact://") {
		return fmt.Errorf("error.protectedStackRef must be an artifact reference")
	}
	return nil
}
