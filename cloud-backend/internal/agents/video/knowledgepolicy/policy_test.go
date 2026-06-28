package knowledgepolicy

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
)

func TestDeciderRequiresNewsForSportsCurrentEvent(t *testing.T) {
	policy := NewDecider().Decide("佛得角世界杯出线是一个奇迹")
	if policy.RetrievalPolicy != agentruntime.RetrievalRequired {
		t.Fatalf("retrieval policy = %q, want required", policy.RetrievalPolicy)
	}
	if policy.KnowledgeType != "latest_news" {
		t.Fatalf("knowledge type = %q, want latest_news", policy.KnowledgeType)
	}
}

func TestDeciderForbidsNewsForOpinion(t *testing.T) {
	policy := NewDecider().Decide("AI 替代的不是岗位，而是工作流程")
	if policy.RetrievalPolicy != agentruntime.RetrievalNone {
		t.Fatalf("retrieval policy = %q, want none", policy.RetrievalPolicy)
	}
}
