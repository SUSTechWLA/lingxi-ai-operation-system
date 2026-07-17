package knowledgepolicy

import "github.com/tangying-ai/aios-core/internal/core/agentruntime"

type Decider struct{}

func NewDecider() Decider {
	return Decider{}
}

func (d Decider) Decide(topic string) *agentruntime.KnowledgePolicy {
	return agentruntime.DefaultKnowledgePolicy(topic, "video_creation")
}
