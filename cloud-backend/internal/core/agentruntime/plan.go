package agentruntime

type AgentPlan struct {
	Goal       string      `json:"goal"`
	Domain     string      `json:"domain"`
	Mode       string      `json:"mode"`
	Steps      []AgentStep `json:"steps"`
	Budget     AgentBudget `json:"budget"`
	StopPolicy StopPolicy  `json:"stopPolicy"`
}

type AgentStep struct {
	ID              string                 `json:"id"`
	Intent          string                 `json:"intent,omitempty"`
	Tool            string                 `json:"tool"`
	Arguments       map[string]interface{} `json:"arguments"`
	DependsOn       []string               `json:"dependsOn,omitempty"`
	ExpectedOutput  []string               `json:"expectedOutput,omitempty"`
	ProduceArtifact bool                   `json:"produceArtifact,omitempty"`
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
