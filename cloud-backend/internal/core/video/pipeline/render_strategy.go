package pipeline

type RenderStrategyRequest struct {
	Shots        []ShotPlan
	Capabilities CapabilitySnapshot
}

type ShotPlan struct {
	ShotID      string          `json:"shotId"`
	DurationSec float64         `json:"durationSec,omitempty"`
	Elements    []VisualElement `json:"elements,omitempty"`
}

type VisualElement struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	RequiresExactText     bool   `json:"requiresExactText,omitempty"`
	RequiresComplexMotion bool   `json:"requiresComplexMotion,omitempty"`
}

type RenderStrategy struct {
	ArtifactKind         string               `json:"artifactKind"`
	StrategyVersion      string               `json:"strategyVersion"`
	OverallMode          string               `json:"overallMode"`
	Shots                []ShotRenderStrategy `json:"shots"`
	FinalAssemblyEngine  string               `json:"finalAssemblyEngine"`
	EstimatedCost        EstimatedRenderCost  `json:"estimatedCost"`
	RequiresUserApproval bool                 `json:"requiresUserApproval"`
	DecisionLog          DecisionLog          `json:"decisionLog"`
}

type ShotRenderStrategy struct {
	ShotID                  string                  `json:"shotId"`
	Engine                  string                  `json:"engine"`
	Reason                  string                  `json:"reason"`
	RiskLevel               string                  `json:"riskLevel"`
	RequiresConsistencyPack bool                    `json:"requiresConsistencyPack,omitempty"`
	MaxDurationSec          int                     `json:"maxDurationSec,omitempty"`
	Elements                []ElementRenderDecision `json:"elements,omitempty"`
}

type ElementRenderDecision struct {
	ElementID string `json:"elementId"`
	Engine    string `json:"engine"`
	Reason    string `json:"reason"`
}

type EstimatedRenderCost struct {
	SeedanceClips      int    `json:"seedanceClips"`
	HyperFramesRenders int    `json:"hyperframesRenders"`
	CostLevel          string `json:"costLevel"`
}

func BuildRenderStrategy(req RenderStrategyRequest) RenderStrategy {
	shots := make([]ShotRenderStrategy, 0, len(req.Shots))
	seedanceClips := 0
	hyperframesRenders := 1
	for _, shot := range req.Shots {
		shotStrategy := planShot(shot, req.Capabilities)
		if shotStrategy.Engine == EngineSeedance || shotStrategy.Engine == EngineHybrid {
			seedanceClips++
		}
		shots = append(shots, shotStrategy)
	}
	overallMode := summarizeMode(shots)
	costLevel := "low"
	if seedanceClips > 0 {
		costLevel = "medium"
	}
	if seedanceClips >= 4 {
		costLevel = "high"
	}
	return RenderStrategy{
		ArtifactKind:         ArtifactRenderStrategy,
		StrategyVersion:      "video-render-strategy-v1",
		OverallMode:          overallMode,
		Shots:                shots,
		FinalAssemblyEngine:  EngineHyperFrames,
		EstimatedCost:        EstimatedRenderCost{SeedanceClips: seedanceClips, HyperFramesRenders: hyperframesRenders, CostLevel: costLevel},
		RequiresUserApproval: true,
		DecisionLog: DecisionLog{
			DecisionID:   "decision_render_strategy_001",
			DecisionType: DecisionRenderStrategySelection,
			OptionsConsidered: []DecisionOption{
				{Mode: RenderModeHyperFramesOnly, Cost: "low", Risk: "low", Weakness: "复杂动态画面较弱", Recommendation: overallMode == RenderModeHyperFramesOnly},
				{Mode: RenderModeSeedanceHeavy, Cost: "high", Risk: "high", Weakness: "长视频一致性风险较高", Recommendation: overallMode == RenderModeSeedanceHeavy},
				{Mode: RenderModeHybrid, Cost: "medium", Risk: "medium", Recommendation: overallMode == RenderModeHybrid},
			},
			Selected: overallMode,
		},
	}
}

func planShot(shot ShotPlan, caps CapabilitySnapshot) ShotRenderStrategy {
	hasExactText := false
	hasComplexMotion := false
	elements := make([]ElementRenderDecision, 0, len(shot.Elements))
	for _, element := range shot.Elements {
		engine := EngineHyperFrames
		reason := "文字、字幕、卡片或确定性排版适合 HyperFrames。"
		if element.RequiresComplexMotion && caps.SeedanceAvailable {
			engine = EngineSeedance
			reason = "复杂自然动态画面更适合生成式视频。"
		}
		if element.RequiresExactText {
			engine = EngineHyperFrames
			reason = "需要准确文字，适合 HyperFrames 确定性合成。"
		}
		elements = append(elements, ElementRenderDecision{ElementID: element.ID, Engine: engine, Reason: reason})
		hasExactText = hasExactText || element.RequiresExactText
		hasComplexMotion = hasComplexMotion || element.RequiresComplexMotion
	}
	switch {
	case hasComplexMotion && hasExactText && caps.SeedanceAvailable:
		return ShotRenderStrategy{
			ShotID:                  shot.ShotID,
			Engine:                  EngineHybrid,
			Reason:                  "复杂动态背景由 Seedance 生成，字幕和知识卡片由 HyperFrames 叠加。",
			RiskLevel:               "medium",
			RequiresConsistencyPack: true,
			MaxDurationSec:          8,
			Elements:                elements,
		}
	case hasComplexMotion && caps.SeedanceAvailable:
		return ShotRenderStrategy{
			ShotID:                  shot.ShotID,
			Engine:                  EngineSeedance,
			Reason:                  "镜头主要需求是复杂动态画面，适合 Seedance。",
			RiskLevel:               "medium",
			RequiresConsistencyPack: true,
			MaxDurationSec:          8,
			Elements:                elements,
		}
	default:
		return ShotRenderStrategy{
			ShotID:    shot.ShotID,
			Engine:    EngineHyperFrames,
			Reason:    "镜头以准确文字、字幕、卡片或可控排版为主，适合 HyperFrames。",
			RiskLevel: "low",
			Elements:  elements,
		}
	}
}

func summarizeMode(shots []ShotRenderStrategy) string {
	if len(shots) == 0 {
		return RenderModeHyperFramesOnly
	}
	hasHyperFrames := false
	hasSeedance := false
	for _, shot := range shots {
		switch shot.Engine {
		case EngineHybrid:
			return RenderModeHybrid
		case EngineSeedance:
			hasSeedance = true
		default:
			hasHyperFrames = true
		}
	}
	if hasHyperFrames && hasSeedance {
		return RenderModeHybrid
	}
	if hasSeedance {
		return RenderModeSeedanceHeavy
	}
	return RenderModeHyperFramesOnly
}
