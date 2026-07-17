package agentruntime

type ArtifactPointer struct {
	ArtifactID       string   `json:"artifactId,omitempty"`
	Kind             string   `json:"kind"`
	Version          int      `json:"version,omitempty"`
	StorageRef       string   `json:"storageRef,omitempty"`
	ProducedByRoleID string   `json:"producedByRoleId,omitempty"`
	ProducedByStage  string   `json:"producedByStage,omitempty"`
	DependsOn        []string `json:"dependsOn,omitempty"`
	HumanApproved    bool     `json:"humanApproved,omitempty"`
	Stale            bool     `json:"stale,omitempty"`
}

type ArtifactIndex struct {
	Artifacts []ArtifactPointer `json:"artifacts"`
}

type RoleMemory struct {
	RoleID       string   `json:"roleId"`
	Stage        string   `json:"stage"`
	Summary      string   `json:"summary"`
	KeyDecisions []string `json:"keyDecisions,omitempty"`
	Constraints  []string `json:"constraints,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
	OpenIssues   []string `json:"openIssues,omitempty"`
	UpdatedAt    string   `json:"updatedAt,omitempty"`
}

type CompactedContext struct {
	ProjectGoal        string            `json:"projectGoal,omitempty"`
	CurrentStage       string            `json:"currentStage,omitempty"`
	CurrentStatus      string            `json:"currentStatus,omitempty"`
	HardConstraints    []string          `json:"hardConstraints,omitempty"`
	KeyDecisions       []string          `json:"keyDecisions,omitempty"`
	ConfirmedArtifacts []ArtifactPointer `json:"confirmedArtifacts,omitempty"`
	StaleArtifacts     []ArtifactPointer `json:"staleArtifacts,omitempty"`
	RoleMemories       []RoleMemory      `json:"roleMemories,omitempty"`
	NextActions        []string          `json:"nextActions,omitempty"`
}

type StageExecutionContext struct {
	ProjectID        string            `json:"projectId,omitempty"`
	RunID            string            `json:"runId,omitempty"`
	Stage            string            `json:"stage"`
	RoleAgentID      string            `json:"roleAgentId"`
	InputArtifacts   []ArtifactPointer `json:"inputArtifacts,omitempty"`
	OutputArtifacts  []ArtifactPointer `json:"outputArtifacts,omitempty"`
	AllowedTools     []string          `json:"allowedTools,omitempty"`
	ForbiddenTools   []string          `json:"forbiddenTools,omitempty"`
	CompactedContext *CompactedContext `json:"compactedContext,omitempty"`
}

type AgentEvent struct {
	RunID       string                 `json:"runId,omitempty"`
	TaskID      string                 `json:"taskId,omitempty"`
	NodeID      string                 `json:"nodeId,omitempty"`
	Stage       string                 `json:"stage,omitempty"`
	RoleAgentID string                 `json:"roleAgentId,omitempty"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}
