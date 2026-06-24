package pipeline

import "fmt"

type CapabilitySnapshot struct {
	TextModelAvailable   bool `json:"textModelAvailable"`
	HyperFramesAvailable bool `json:"hyperframesAvailable"`
	SeedanceAvailable    bool `json:"seedanceAvailable"`
	TTSAvailable         bool `json:"ttsAvailable"`
	ASRAvailable         bool `json:"asrAvailable"`
}

type ProposalRequest struct {
	Pipeline          *Manifest
	Message           string
	TargetDurationSec int
	Capabilities      CapabilitySnapshot
}

type ProposalPacket struct {
	ArtifactKind        string           `json:"artifactKind"`
	PipelineID          string           `json:"pipelineId"`
	PipelineName        string           `json:"pipelineName"`
	Options             []ProposalOption `json:"options"`
	RecommendedOptionID string           `json:"recommendedOptionId"`
	RequiresApproval    bool             `json:"requiresApproval"`
	DecisionLog         DecisionLog      `json:"decisionLog"`
}

type ProposalOption struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	RenderMode           string `json:"renderMode"`
	EstimatedDurationSec int    `json:"estimatedDurationSec"`
	EstimatedCost        string `json:"estimatedCost"`
	Risk                 string `json:"risk"`
}

type DecisionLog struct {
	DecisionID        string           `json:"decisionId"`
	DecisionType      string           `json:"decisionType"`
	OptionsConsidered []DecisionOption `json:"optionsConsidered"`
	Selected          string           `json:"selected"`
	ApprovedByUser    bool             `json:"approvedByUser"`
}

type DecisionOption struct {
	Mode           string `json:"mode"`
	Cost           string `json:"cost"`
	Risk           string `json:"risk"`
	Weakness       string `json:"weakness,omitempty"`
	Recommendation bool   `json:"recommendation,omitempty"`
}

func BuildProposalPacket(req ProposalRequest) ProposalPacket {
	duration := req.TargetDurationSec
	if duration <= 0 {
		duration = 60
	}
	pipelineID := ""
	pipelineName := ""
	if req.Pipeline != nil {
		pipelineID = req.Pipeline.ID
		pipelineName = req.Pipeline.Name
	}

	options := []ProposalOption{
		{
			ID:                   "option_a",
			Name:                 "低成本图文口播版",
			Description:          "主要使用 HyperFrames，少量 AI 图片，成本低、可控性高。",
			RenderMode:           RenderModeHyperFramesOnly,
			EstimatedDurationSec: duration,
			EstimatedCost:        "low",
			Risk:                 "low",
		},
		{
			ID:                   "option_b",
			Name:                 "混合动态版",
			Description:          "Seedance 生成部分 B-roll，HyperFrames 负责字幕和知识卡片。",
			RenderMode:           RenderModeHybrid,
			EstimatedDurationSec: duration,
			EstimatedCost:        "medium",
			Risk:                 "medium",
		},
		{
			ID:                   "option_c",
			Name:                 "电影感动态版",
			Description:          "较多使用 Seedance，画面丰富，但一致性和成本风险更高。",
			RenderMode:           RenderModeSeedanceHeavy,
			EstimatedDurationSec: duration,
			EstimatedCost:        "high",
			Risk:                 "high",
		},
	}

	recommended := "option_a"
	if req.Capabilities.SeedanceAvailable {
		recommended = "option_b"
	}
	return ProposalPacket{
		ArtifactKind:        ArtifactProposalPacket,
		PipelineID:          pipelineID,
		PipelineName:        pipelineName,
		Options:             options,
		RecommendedOptionID: recommended,
		RequiresApproval:    true,
		DecisionLog: DecisionLog{
			DecisionID:   fmt.Sprintf("decision_%s_001", DecisionProposalSelection),
			DecisionType: DecisionProposalSelection,
			OptionsConsidered: []DecisionOption{
				{Mode: RenderModeHyperFramesOnly, Cost: "low", Risk: "low", Weakness: "复杂动态画面较弱", Recommendation: recommended == "option_a"},
				{Mode: RenderModeHybrid, Cost: "medium", Risk: "medium", Recommendation: recommended == "option_b"},
				{Mode: RenderModeSeedanceHeavy, Cost: "high", Risk: "high", Weakness: "长视频一致性风险较高", Recommendation: recommended == "option_c"},
			},
			Selected: recommended,
		},
	}
}
