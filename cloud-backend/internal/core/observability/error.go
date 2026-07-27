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

var errorRegistry = map[string]errorDefinition{
	"AUTH.SESSION.EXPIRED": {
		Class:              ErrorClassAuthentication,
		Retryable:          false,
		UserMessageKey:     "error.auth.session.expired",
		SuggestedActionKey: "action.auth.sign.in",
	},
	"WORKFLOW.STAGE.TIMEOUT": {
		Class:              ErrorClassTimeout,
		Retryable:          true,
		UserMessageKey:     "error.workflow.stage.timeout",
		SuggestedActionKey: "action.workflow.retry.stage",
	},
	"AGENT.PLAN.VALIDATION_FAILED": {
		Class:              ErrorClassValidation,
		Retryable:          false,
		UserMessageKey:     "error.agent.plan.validation.failed",
		SuggestedActionKey: "action.agent.review.plan",
	},
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

	return &EventError{
		Code:               code,
		Class:              definition.Class,
		Fingerprint:        hex.EncodeToString(sum[:]),
		Retryable:          definition.Retryable,
		CausedByEventID:    causedBy,
		UserMessageKey:     definition.UserMessageKey,
		DeveloperDetail:    diagnosticKey(code),
		SuggestedActionKey: definition.SuggestedActionKey,
	}
}

func diagnosticKey(code string) string {
	key := strings.ToLower(strings.ReplaceAll(code, "_", "."))
	return "diagnostic." + key
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
	if e.CausedByEventID != "" && !eventIDPattern.MatchString(e.CausedByEventID) {
		return fmt.Errorf("error.causedByEventId has an invalid format")
	}
	if e.DeveloperDetail != "" && !developerDetailPattern.MatchString(e.DeveloperDetail) {
		return fmt.Errorf("error.developerDetail must be a stable diagnostic key")
	}
	if err := validateArtifactRefs("error.evidenceRefs", e.EvidenceRefs); err != nil {
		return err
	}
	if e.ProtectedStackRef != "" && !strings.HasPrefix(e.ProtectedStackRef, "artifact://") {
		return fmt.Errorf("error.protectedStackRef must be an artifact reference")
	}
	return nil
}
