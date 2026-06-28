package agentruntime

type AgentPlan struct {
	Goal            string           `json:"goal"`
	Domain          string           `json:"domain"`
	Mode            string           `json:"mode"`
	KnowledgePolicy *KnowledgePolicy `json:"knowledgePolicy,omitempty"`
	ToolTrace       *ToolTrace       `json:"toolTrace,omitempty"`
	Steps           []AgentStep      `json:"steps"`
	Budget          AgentBudget      `json:"budget"`
	StopPolicy      StopPolicy       `json:"stopPolicy"`
}

type AgentStep struct {
	ID              string                 `json:"id"`
	Intent          string                 `json:"intent,omitempty"`
	Tool            string                 `json:"tool"`
	Reason          string                 `json:"reason,omitempty"`
	Arguments       map[string]interface{} `json:"arguments"`
	DependsOn       []string               `json:"dependsOn,omitempty"`
	ExpectedOutput  []string               `json:"expectedOutput,omitempty"`
	ProduceArtifact bool                   `json:"produceArtifact,omitempty"`
}

type ToolTrace struct {
	CandidateTools   []ToolCandidateTrace  `json:"candidateTools,omitempty"`
	PlannedTools     []string              `json:"plannedTools,omitempty"`
	ExecutedTools    []string              `json:"executedTools,omitempty"`
	GuardDecision    *GuardDecisionTrace   `json:"guardDecision,omitempty"`
	KnowledgeContext *KnowledgeContextInfo `json:"knowledgeContext,omitempty"`
}

type ToolCandidateTrace struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type GuardDecisionTrace struct {
	Passed   bool     `json:"passed"`
	Warnings []string `json:"warnings,omitempty"`
}

type KnowledgeContextInfo struct {
	ItemCount   int      `json:"itemCount"`
	SourceCount int      `json:"sourceCount"`
	GeneratedBy []string `json:"generatedBy,omitempty"`
}

type AgentBudget struct {
	MaxLLMCalls  int    `json:"maxLLMCalls,omitempty"`
	MaxToolCalls int    `json:"maxToolCalls,omitempty"`
	MaxSteps     int    `json:"maxSteps,omitempty"`
	MaxReplans   int    `json:"maxReplans,omitempty"`
	MaxCostLevel string `json:"maxCostLevel,omitempty"`
}

type StopPolicy struct {
	StopWhenEnough bool `json:"stopWhenEnough,omitempty"`
}
