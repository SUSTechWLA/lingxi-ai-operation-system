package observability

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Severity string

const (
	SeverityDebug Severity = "DEBUG"
	SeverityInfo  Severity = "INFO"
	SeverityWarn  Severity = "WARN"
	SeverityError Severity = "ERROR"
)

type EventType string

const (
	EventTypeRequestAccepted                EventType = "request.accepted"
	EventTypeRequestAuthenticationSucceeded EventType = "request.authentication.succeeded"
	EventTypeRequestAuthenticationFailed    EventType = "request.authentication.failed"
	EventTypeTaskCreated                    EventType = "task.created"
	EventTypeTaskCancellationRequested      EventType = "task.cancellation.requested"
	EventTypeTaskCompleted                  EventType = "task.completed"
	EventTypeTaskFailed                     EventType = "task.failed"
	EventTypeTaskCancelled                  EventType = "task.cancelled"
	EventTypeRunManifestCreated             EventType = "run.manifest.created"
	EventTypeWorkflowRunQueued              EventType = "workflow.run.queued"
	EventTypeWorkflowRunStarted             EventType = "workflow.run.started"
	EventTypeWorkflowRunCompleted           EventType = "workflow.run.completed"
	EventTypeWorkflowRunFailed              EventType = "workflow.run.failed"
	EventTypeWorkflowRunCancelled           EventType = "workflow.run.cancelled"
	EventTypeWorkflowStageQueued            EventType = "workflow.stage.queued"
	EventTypeWorkflowStageStarted           EventType = "workflow.stage.started"
	EventTypeWorkflowStageProgress          EventType = "workflow.stage.progress"
	EventTypeWorkflowStageCheckpointed      EventType = "workflow.stage.checkpointed"
	EventTypeWorkflowStagePaused            EventType = "workflow.stage.paused"
	EventTypeWorkflowStageResumed           EventType = "workflow.stage.resumed"
	EventTypeWorkflowStageCompleted         EventType = "workflow.stage.completed"
	EventTypeWorkflowStageFailed            EventType = "workflow.stage.failed"
	EventTypeWorkflowStageCancelled         EventType = "workflow.stage.cancelled"
	EventTypeAgentRunStarted                EventType = "agent.run.started"
	EventTypeAgentRunCompleted              EventType = "agent.run.completed"
	EventTypeAgentRunFailed                 EventType = "agent.run.failed"
	EventTypeAgentRunCancelled              EventType = "agent.run.cancelled"
	EventTypeAgentPlanValidationStarted     EventType = "agent.plan.validation.started"
	EventTypeAgentPlanValidationCompleted   EventType = "agent.plan.validation.completed"
	EventTypeAgentPlanValidationFailed      EventType = "agent.plan.validation.failed"
	EventTypeAgentDecisionRecorded          EventType = "agent.decision.recorded"
	EventTypeAgentResultSubmitted           EventType = "agent.result.submitted"
	EventTypeLLMCallStarted                 EventType = "llm.call.started"
	EventTypeLLMCallCompleted               EventType = "llm.call.completed"
	EventTypeLLMCallFailed                  EventType = "llm.call.failed"
	EventTypeMCPConnectionStarted           EventType = "mcp.connection.started"
	EventTypeMCPConnectionCompleted         EventType = "mcp.connection.completed"
	EventTypeMCPConnectionFailed            EventType = "mcp.connection.failed"
	EventTypeMCPDiscoveryStarted            EventType = "mcp.discovery.started"
	EventTypeMCPDiscoveryCompleted          EventType = "mcp.discovery.completed"
	EventTypeMCPDiscoveryFailed             EventType = "mcp.discovery.failed"
	EventTypeMCPToolCallStarted             EventType = "mcp.tool.call.started"
	EventTypeMCPToolCallCompleted           EventType = "mcp.tool.call.completed"
	EventTypeMCPToolCallFailed              EventType = "mcp.tool.call.failed"
	EventTypeToolCallStarted                EventType = "tool.call.started"
	EventTypeToolCallCompleted              EventType = "tool.call.completed"
	EventTypeToolCallFailed                 EventType = "tool.call.failed"
	EventTypeLocalJobStarted                EventType = "local.job.started"
	EventTypeLocalJobCompleted              EventType = "local.job.completed"
	EventTypeLocalJobFailed                 EventType = "local.job.failed"
	EventTypeVerifyCheckStarted             EventType = "verify.check.started"
	EventTypeVerifyCheckPassed              EventType = "verify.check.passed"
	EventTypeVerifyCheckWarned              EventType = "verify.check.warned"
	EventTypeVerifyCheckFailed              EventType = "verify.check.failed"
	EventTypeCorrectOperationStarted        EventType = "correct.operation.started"
	EventTypeCorrectOperationCompleted      EventType = "correct.operation.completed"
	EventTypeCorrectOperationFailed         EventType = "correct.operation.failed"
	EventTypeRecoveryRetryStarted           EventType = "recovery.retry.started"
	EventTypeRecoveryRetryCompleted         EventType = "recovery.retry.completed"
	EventTypeRecoveryRetryFailed            EventType = "recovery.retry.failed"
	EventTypeRecoveryFallbackStarted        EventType = "recovery.fallback.started"
	EventTypeRecoveryFallbackCompleted      EventType = "recovery.fallback.completed"
	EventTypeRecoveryFallbackFailed         EventType = "recovery.fallback.failed"
	EventTypeArtifactCreated                EventType = "artifact.created"
	EventTypeArtifactValidated              EventType = "artifact.validated"
	EventTypeArtifactValidationFailed       EventType = "artifact.validation.failed"
	EventTypeArtifactMaterialized           EventType = "artifact.materialized"
	EventTypeArtifactPreviewAvailable       EventType = "artifact.preview.available"
	EventTypeArtifactExported               EventType = "artifact.exported"
	EventTypeUserDecisionRecorded           EventType = "user.decision.recorded"
)

type ExecutionStatus string

const (
	ExecutionStatusQueued     ExecutionStatus = "QUEUED"
	ExecutionStatusStarted    ExecutionStatus = "STARTED"
	ExecutionStatusInProgress ExecutionStatus = "IN_PROGRESS"
	ExecutionStatusCompleted  ExecutionStatus = "COMPLETED"
	ExecutionStatusFailed     ExecutionStatus = "FAILED"
	ExecutionStatusCancelled  ExecutionStatus = "CANCELLED"
	ExecutionStatusPaused     ExecutionStatus = "PAUSED"
	ExecutionStatusSkipped    ExecutionStatus = "SKIPPED"
)

type PrivacyClassification string

const (
	PrivacyPublic    PrivacyClassification = "PUBLIC"
	PrivacyInternal  PrivacyClassification = "INTERNAL"
	PrivacySensitive PrivacyClassification = "SENSITIVE"
	PrivacySecret    PrivacyClassification = "SECRET"
)

type Event struct {
	SchemaVersion    string      `json:"schemaVersion"`
	EventID          string      `json:"eventId"`
	OccurredAt       time.Time   `json:"occurredAt"`
	IngestedAt       time.Time   `json:"ingestedAt"`
	ProducerSequence int64       `json:"producerSequence"`
	Severity         Severity    `json:"severity"`
	EventType        EventType   `json:"eventType"`
	MessageKey       string      `json:"messageKey"`
	Source           Source      `json:"source"`
	Correlation      Correlation `json:"correlation"`
	Execution        Execution   `json:"execution"`
	Runtime          Runtime     `json:"runtime"`
	Evidence         Evidence    `json:"evidence"`
	Error            *EventError `json:"error"`
	Privacy          Privacy     `json:"privacy"`
}

type Source struct {
	Service     string `json:"service"`
	Component   string `json:"component"`
	Environment string `json:"environment"`
}

type Correlation struct {
	TraceID       string `json:"traceId"`
	SpanID        string `json:"spanId"`
	ParentSpanID  string `json:"parentSpanId,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	TaskID        string `json:"taskId,omitempty"`
	WorkflowRunID string `json:"workflowRunId,omitempty"`
	AgentRunID    string `json:"agentRunId,omitempty"`
	StageID       string `json:"stageId,omitempty"`
	ShotID        string `json:"shotId,omitempty"`
	ArtifactID    string `json:"artifactId,omitempty"`
	ToolCallID    string `json:"toolCallId,omitempty"`
	ProviderJobID string `json:"providerJobId,omitempty"`
}

type Execution struct {
	Status     ExecutionStatus `json:"status"`
	Attempt    int64           `json:"attempt"`
	DurationMs *int64          `json:"durationMs,omitempty"`
}

type Runtime struct {
	AppVersion             string `json:"appVersion,omitempty"`
	GitCommit              string `json:"gitCommit,omitempty"`
	WorkflowVersion        string `json:"workflowVersion,omitempty"`
	ToolRegistrySnapshotID string `json:"toolRegistrySnapshotId,omitempty"`
	PromptTemplateVersion  string `json:"promptTemplateVersion,omitempty"`
	Provider               string `json:"provider,omitempty"`
	Model                  string `json:"model,omitempty"`
}

type Evidence struct {
	InputRefs  []string `json:"inputRefs,omitempty"`
	OutputRefs []string `json:"outputRefs,omitempty"`
	InputHash  string   `json:"inputHash,omitempty"`
	OutputHash string   `json:"outputHash,omitempty"`
	SizeBytes  *int64   `json:"sizeBytes,omitempty"`

	// Attributes is accepted only as pre-redaction input. The v1 wire contract
	// is closed, so arbitrary attributes are never serialized.
	Attributes map[string]any `json:"-"`
}

type Privacy struct {
	Classification PrivacyClassification `json:"classification"`
	RedactedFields []string              `json:"redactedFields"`
}

var (
	eventIDPattern         = regexp.MustCompile(`^evt_[A-Za-z0-9_-]+$`)
	traceIDPattern         = regexp.MustCompile(`^trc_[A-Za-z0-9_-]+$`)
	spanIDPattern          = regexp.MustCompile(`^spn_[A-Za-z0-9_-]+$`)
	sessionIDPattern       = regexp.MustCompile(`^ses_[A-Za-z0-9_-]+$`)
	projectIDPattern       = regexp.MustCompile(`^prj_[A-Za-z0-9_-]+$`)
	taskIDPattern          = regexp.MustCompile(`^tsk_[A-Za-z0-9_-]+$`)
	workflowRunIDPattern   = regexp.MustCompile(`^wfr_[A-Za-z0-9_-]+$`)
	agentRunIDPattern      = regexp.MustCompile(`^agr_[A-Za-z0-9_-]+$`)
	shotIDPattern          = regexp.MustCompile(`^shot_[A-Za-z0-9_-]+$`)
	artifactIDPattern      = regexp.MustCompile(`^art_[A-Za-z0-9_-]+$`)
	toolCallIDPattern      = regexp.MustCompile(`^call_[A-Za-z0-9_-]+$`)
	providerJobIDPattern   = regexp.MustCompile(`^job_[A-Za-z0-9_-]+$`)
	messageKeyPattern      = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+$`)
	hashPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	developerDetailPattern = regexp.MustCompile(`^diagnostic\.[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+$`)
)

var eventTypes = map[EventType]struct{}{
	EventTypeRequestAccepted:                {},
	EventTypeRequestAuthenticationSucceeded: {},
	EventTypeRequestAuthenticationFailed:    {},
	EventTypeTaskCreated:                    {},
	EventTypeTaskCancellationRequested:      {},
	EventTypeTaskCompleted:                  {},
	EventTypeTaskFailed:                     {},
	EventTypeTaskCancelled:                  {},
	EventTypeRunManifestCreated:             {},
	EventTypeWorkflowRunQueued:              {},
	EventTypeWorkflowRunStarted:             {},
	EventTypeWorkflowRunCompleted:           {},
	EventTypeWorkflowRunFailed:              {},
	EventTypeWorkflowRunCancelled:           {},
	EventTypeWorkflowStageQueued:            {},
	EventTypeWorkflowStageStarted:           {},
	EventTypeWorkflowStageProgress:          {},
	EventTypeWorkflowStageCheckpointed:      {},
	EventTypeWorkflowStagePaused:            {},
	EventTypeWorkflowStageResumed:           {},
	EventTypeWorkflowStageCompleted:         {},
	EventTypeWorkflowStageFailed:            {},
	EventTypeWorkflowStageCancelled:         {},
	EventTypeAgentRunStarted:                {},
	EventTypeAgentRunCompleted:              {},
	EventTypeAgentRunFailed:                 {},
	EventTypeAgentRunCancelled:              {},
	EventTypeAgentPlanValidationStarted:     {},
	EventTypeAgentPlanValidationCompleted:   {},
	EventTypeAgentPlanValidationFailed:      {},
	EventTypeAgentDecisionRecorded:          {},
	EventTypeAgentResultSubmitted:           {},
	EventTypeLLMCallStarted:                 {},
	EventTypeLLMCallCompleted:               {},
	EventTypeLLMCallFailed:                  {},
	EventTypeMCPConnectionStarted:           {},
	EventTypeMCPConnectionCompleted:         {},
	EventTypeMCPConnectionFailed:            {},
	EventTypeMCPDiscoveryStarted:            {},
	EventTypeMCPDiscoveryCompleted:          {},
	EventTypeMCPDiscoveryFailed:             {},
	EventTypeMCPToolCallStarted:             {},
	EventTypeMCPToolCallCompleted:           {},
	EventTypeMCPToolCallFailed:              {},
	EventTypeToolCallStarted:                {},
	EventTypeToolCallCompleted:              {},
	EventTypeToolCallFailed:                 {},
	EventTypeLocalJobStarted:                {},
	EventTypeLocalJobCompleted:              {},
	EventTypeLocalJobFailed:                 {},
	EventTypeVerifyCheckStarted:             {},
	EventTypeVerifyCheckPassed:              {},
	EventTypeVerifyCheckWarned:              {},
	EventTypeVerifyCheckFailed:              {},
	EventTypeCorrectOperationStarted:        {},
	EventTypeCorrectOperationCompleted:      {},
	EventTypeCorrectOperationFailed:         {},
	EventTypeRecoveryRetryStarted:           {},
	EventTypeRecoveryRetryCompleted:         {},
	EventTypeRecoveryRetryFailed:            {},
	EventTypeRecoveryFallbackStarted:        {},
	EventTypeRecoveryFallbackCompleted:      {},
	EventTypeRecoveryFallbackFailed:         {},
	EventTypeArtifactCreated:                {},
	EventTypeArtifactValidated:              {},
	EventTypeArtifactValidationFailed:       {},
	EventTypeArtifactMaterialized:           {},
	EventTypeArtifactPreviewAvailable:       {},
	EventTypeArtifactExported:               {},
	EventTypeUserDecisionRecorded:           {},
}

var severities = map[Severity]struct{}{
	SeverityDebug: {},
	SeverityInfo:  {},
	SeverityWarn:  {},
	SeverityError: {},
}

var executionStatuses = map[ExecutionStatus]struct{}{
	ExecutionStatusQueued:     {},
	ExecutionStatusStarted:    {},
	ExecutionStatusInProgress: {},
	ExecutionStatusCompleted:  {},
	ExecutionStatusFailed:     {},
	ExecutionStatusCancelled:  {},
	ExecutionStatusPaused:     {},
	ExecutionStatusSkipped:    {},
}

var privacyClassifications = map[PrivacyClassification]struct{}{
	PrivacyPublic:    {},
	PrivacyInternal:  {},
	PrivacySensitive: {},
	PrivacySecret:    {},
}

func (e Event) Validate() error {
	if e.SchemaVersion != "1.0" {
		return fmt.Errorf("schemaVersion must be 1.0")
	}
	if !eventIDPattern.MatchString(e.EventID) {
		return fmt.Errorf("eventId has an invalid format")
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("occurredAt is required")
	}
	if e.IngestedAt.IsZero() {
		return fmt.Errorf("ingestedAt is required")
	}
	if e.ProducerSequence < 0 {
		return fmt.Errorf("producerSequence must be non-negative")
	}
	if _, ok := severities[e.Severity]; !ok {
		return fmt.Errorf("severity is not registered")
	}
	if _, ok := eventTypes[e.EventType]; !ok {
		return fmt.Errorf("eventType is not registered")
	}
	if !messageKeyPattern.MatchString(e.MessageKey) {
		return fmt.Errorf("messageKey has an invalid format")
	}
	if strings.TrimSpace(e.Source.Service) == "" {
		return fmt.Errorf("source.service is required")
	}
	if strings.TrimSpace(e.Source.Component) == "" {
		return fmt.Errorf("source.component is required")
	}
	if strings.TrimSpace(e.Source.Environment) == "" {
		return fmt.Errorf("source.environment is required")
	}
	if err := e.Correlation.validate(); err != nil {
		return err
	}
	if err := e.Execution.validate(); err != nil {
		return err
	}
	if err := e.Evidence.validate(); err != nil {
		return err
	}
	if e.Severity == SeverityError && e.Error == nil {
		return fmt.Errorf("error is required for ERROR severity")
	}
	if e.Error != nil {
		if err := e.Error.validate(); err != nil {
			return err
		}
	}
	if _, ok := privacyClassifications[e.Privacy.Classification]; !ok {
		return fmt.Errorf("privacy.classification is not registered")
	}
	if e.Privacy.RedactedFields == nil {
		return fmt.Errorf("privacy.redactedFields is required")
	}
	seen := make(map[string]struct{}, len(e.Privacy.RedactedFields))
	for _, field := range e.Privacy.RedactedFields {
		if strings.TrimSpace(field) == "" {
			return fmt.Errorf("privacy.redactedFields must not contain empty values")
		}
		if _, ok := seen[field]; ok {
			return fmt.Errorf("privacy.redactedFields must contain unique values")
		}
		seen[field] = struct{}{}
	}
	return nil
}

func (c Correlation) validate() error {
	checks := []struct {
		name     string
		value    string
		pattern  *regexp.Regexp
		required bool
	}{
		{"correlation.traceId", c.TraceID, traceIDPattern, true},
		{"correlation.spanId", c.SpanID, spanIDPattern, true},
		{"correlation.parentSpanId", c.ParentSpanID, spanIDPattern, false},
		{"correlation.sessionId", c.SessionID, sessionIDPattern, false},
		{"correlation.projectId", c.ProjectID, projectIDPattern, false},
		{"correlation.taskId", c.TaskID, taskIDPattern, false},
		{"correlation.workflowRunId", c.WorkflowRunID, workflowRunIDPattern, false},
		{"correlation.agentRunId", c.AgentRunID, agentRunIDPattern, false},
		{"correlation.shotId", c.ShotID, shotIDPattern, false},
		{"correlation.artifactId", c.ArtifactID, artifactIDPattern, false},
		{"correlation.toolCallId", c.ToolCallID, toolCallIDPattern, false},
		{"correlation.providerJobId", c.ProviderJobID, providerJobIDPattern, false},
	}
	for _, check := range checks {
		if check.value == "" && !check.required {
			continue
		}
		if !check.pattern.MatchString(check.value) {
			return fmt.Errorf("%s has an invalid format", check.name)
		}
	}
	if c.StageID != "" && strings.TrimSpace(c.StageID) == "" {
		return fmt.Errorf("correlation.stageId must not be blank")
	}
	return nil
}

func (e Execution) validate() error {
	if _, ok := executionStatuses[e.Status]; !ok {
		return fmt.Errorf("execution.status is not registered")
	}
	if e.Attempt < 1 {
		return fmt.Errorf("execution.attempt must be at least 1")
	}
	if e.DurationMs != nil && *e.DurationMs < 0 {
		return fmt.Errorf("execution.durationMs must be non-negative")
	}
	return nil
}

func (e Evidence) validate() error {
	if err := validateArtifactRefs("evidence.inputRefs", e.InputRefs); err != nil {
		return err
	}
	if err := validateArtifactRefs("evidence.outputRefs", e.OutputRefs); err != nil {
		return err
	}
	if e.InputHash != "" && !hashPattern.MatchString(e.InputHash) {
		return fmt.Errorf("evidence.inputHash has an invalid format")
	}
	if e.OutputHash != "" && !hashPattern.MatchString(e.OutputHash) {
		return fmt.Errorf("evidence.outputHash has an invalid format")
	}
	if e.SizeBytes != nil && *e.SizeBytes < 0 {
		return fmt.Errorf("evidence.sizeBytes must be non-negative")
	}
	return nil
}

func validateArtifactRefs(field string, refs []string) error {
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "artifact://") {
			return fmt.Errorf("%s contains an invalid artifact reference", field)
		}
	}
	return nil
}
