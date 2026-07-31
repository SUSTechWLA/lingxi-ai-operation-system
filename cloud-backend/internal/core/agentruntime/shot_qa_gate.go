package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoservice "github.com/tangying-ai/aios-core/internal/agents/video/service"
)

// ShotQAResult summarizes what happened after processing a video_frame_qa report.
type ShotQAResult struct {
	// Passed is true when all shots passed QA or were human-approved.
	Passed bool `json:"passed"`

	// NeedsRepair is true when at least one shot needs repair.
	NeedsRepair bool `json:"needsRepair"`

	// NeedsRegeneration is true when at least one shot needs AIGC regeneration.
	NeedsRegeneration bool `json:"needsRegeneration"`

	// NeedsRecomposite is true when at least one shot needs FFmpeg recompositing.
	NeedsRecomposite bool `json:"needsRecomposite"`

	// RepairPlans contains the repair plans for failed shots.
	RepairPlans []videomodel.RepairPlan `json:"repairPlans"`

	// AcceptedCount is the number of shots that passed QA and were accepted.
	AcceptedCount int `json:"acceptedCount"`

	// FailedCount is the number of shots that failed QA.
	FailedCount int `json:"failedCount"`

	// HumanReviewCount is the number of shots that need human review.
	HumanReviewCount int `json:"humanReviewCount"`

	// AssemblyPlan is populated when all shots are accepted.
	AssemblyPlan *videomodel.FinalAssemblyPlan `json:"assemblyPlan,omitempty"`

	// AssemblyIssues are validation issues preventing final assembly.
	AssemblyIssues []videoservice.ValidationIssue `json:"assemblyIssues,omitempty"`
}

// ShotRepairService defines the subset of the video shot repair service
// needed by the shot QA gate.
type ShotRepairService interface {
	ProcessShotQAResult(
		ctx context.Context,
		shots []videomodel.ShotUnit,
		candidates []videomodel.ShotCandidate,
		reports []videomodel.ShotQAReport,
	) ([]videomodel.ShotUnit, error)

	BuildAssemblyPlan(
		shots []videomodel.ShotUnit,
	) (videomodel.FinalAssemblyPlan, []videoservice.ValidationIssue)
}

// shotRepairAdapter adapts the video service package functions to the
// ShotRepairService interface for testability.
type shotRepairAdapter struct{}

func (a *shotRepairAdapter) ProcessShotQAResult(
	_ context.Context,
	shots []videomodel.ShotUnit,
	candidates []videomodel.ShotCandidate,
	reports []videomodel.ShotQAReport,
) ([]videomodel.ShotUnit, error) {
	policy := videomodel.DefaultShotRepairPolicy()
	result := make([]videomodel.ShotUnit, 0, len(shots))
	for i, shot := range shots {
		if i >= len(reports) || i >= len(candidates) {
			result = append(result, shot)
			continue
		}
		updated := videoservice.RecordShotCandidateQA(shot, candidates[i], reports[i], policy)
		result = append(result, updated)
	}
	return result, nil
}

func (a *shotRepairAdapter) BuildAssemblyPlan(
	shots []videomodel.ShotUnit,
) (videomodel.FinalAssemblyPlan, []videoservice.ValidationIssue) {
	return videoservice.BuildFinalAssemblyPlan(shots)
}

// ShotQAGate processes video_frame_qa tool outputs and enforces the shot
// acceptance contract: passed shots are accepted, failed shots get repair
// plans, and final assembly is validated when all shots are ready.
type ShotQAGate struct {
	service ShotRepairService
}

// NewShotQAGate creates a shot QA gate with the default service adapter.
func NewShotQAGate() *ShotQAGate {
	return &ShotQAGate{service: &shotRepairAdapter{}}
}

// NewShotQAGateWithService creates a shot QA gate with a custom service
// implementation (useful for testing).
func NewShotQAGateWithService(service ShotRepairService) *ShotQAGate {
	return &ShotQAGate{service: service}
}

// ProcessVisualQAResult reads the output from a video_frame_qa tool run and
// enforces the shot acceptance contract. It returns a structured result
// indicating whether shots passed, need repair, or need human review.
func (g *ShotQAGate) ProcessVisualQAResult(
	ctx context.Context,
	shots []videomodel.ShotUnit,
	qaRawOutput map[string]interface{},
) (*ShotQAResult, error) {
	if g == nil || g.service == nil {
		return &ShotQAResult{Passed: true}, nil
	}

	candidates, reports, err := extractShotQAData(shots, qaRawOutput)
	if err != nil {
		return nil, fmt.Errorf("extract shot QA data: %w", err)
	}
	if len(reports) == 0 {
		zap.L().Warn("video_frame_qa output contained no shot reports",
			zap.Int("shot_count", len(shots)),
		)
		return &ShotQAResult{Passed: true}, nil
	}

	processed, err := g.service.ProcessShotQAResult(ctx, shots, candidates, reports)
	if err != nil {
		return nil, fmt.Errorf("process shot QA result: %w", err)
	}

	result := &ShotQAResult{}
	for _, shot := range processed {
		switch shot.QAStatus {
		case videomodel.ShotAcceptedForAssembly:
			result.AcceptedCount++
		case videomodel.ShotQAFailed:
			result.FailedCount++
			result.NeedsRepair = true
		case videomodel.ShotHumanReviewRequired:
			result.HumanReviewCount++
		}
		for _, plan := range shot.RepairPlans {
			result.RepairPlans = append(result.RepairPlans, plan)
		}
	}

	if result.FailedCount > 0 || result.HumanReviewCount > 0 {
		result.Passed = false
	} else {
		result.Passed = result.AcceptedCount > 0
	}

	// Determine regeneration needs from repair plans.
	for _, plan := range result.RepairPlans {
		switch plan.Action {
		case videomodel.RepairActionRegenAIGC,
			videomodel.RepairActionRegenAIGCWithReference,
			videomodel.RepairActionPromptPatchRegen:
			result.NeedsRegeneration = true
		case videomodel.RepairActionRecomposite,
			videomodel.RepairActionRerenderText,
			videomodel.RepairActionReencode:
			result.NeedsRecomposite = true
		}
	}

	// If all shots are accepted, build the assembly plan.
	if result.Passed {
		assembly, issues := g.service.BuildAssemblyPlan(processed)
		result.AssemblyPlan = &assembly
		result.AssemblyIssues = issues
		if len(issues) > 0 {
			result.Passed = false
			zap.L().Warn("final assembly plan has validation issues",
				zap.Int("issue_count", len(issues)),
			)
		}
	}

	zap.L().Info("shot QA gate processed visual QA result",
		zap.Bool("passed", result.Passed),
		zap.Int("accepted", result.AcceptedCount),
		zap.Int("failed", result.FailedCount),
		zap.Int("human_review", result.HumanReviewCount),
		zap.Bool("needs_repair", result.NeedsRepair),
		zap.Bool("needs_regeneration", result.NeedsRegeneration),
		zap.Bool("needs_recomposite", result.NeedsRecomposite),
	)

	return result, nil
}

// extractShotQAData extracts shot candidate and QA report data from a
// video_frame_qa MCP tool output payload.
func extractShotQAData(
	shots []videomodel.ShotUnit,
	raw map[string]interface{},
) ([]videomodel.ShotCandidate, []videomodel.ShotQAReport, error) {
	if raw == nil {
		return nil, nil, nil
	}

	// The video_frame_qa MCP tool returns a result with shotReports, shotSummaries,
	// and a global repairPlan. Each shot report maps to a shot candidate.
	shotReportsRaw, _ := raw["shotReports"].([]interface{})
	if shotReportsRaw == nil {
		// Try nested under "result" or "output"
		if nested, ok := raw["result"].(map[string]interface{}); ok {
			shotReportsRaw, _ = nested["shotReports"].([]interface{})
		}
		if nested, ok := raw["output"].(map[string]interface{}); ok {
			if shotReportsRaw == nil {
				shotReportsRaw, _ = nested["shotReports"].([]interface{})
			}
		}
	}

	candidates := make([]videomodel.ShotCandidate, 0, len(shotReportsRaw))
	reports := make([]videomodel.ShotQAReport, 0, len(shotReportsRaw))

	for i, reportRaw := range shotReportsRaw {
		reportMap, ok := reportRaw.(map[string]interface{})
		if !ok {
			continue
		}

		report, err := parseShotQAReport(reportMap)
		if err != nil {
			zap.L().Warn("failed to parse shot QA report",
				zap.Int("index", i),
				zap.Error(err),
			)
			continue
		}

		shotID := report.ShotID
		if shotID == "" && i < len(shots) {
			shotID = shots[i].ID
		}

		candidate := videomodel.ShotCandidate{
			ShotID:       shotID,
			CandidateID:  report.CandidateID,
			AttemptIndex: 0,
			Status:       videomodel.CandidateRendered,
			SourceType:   sourceTypeForReport(report),
			IsFallback:   false,
		}

		// If we have a shot, use its duration and existing candidate index.
		if i < len(shots) {
			candidate.AttemptIndex = len(shots[i].Candidates)
			if candidate.AttemptIndex > 0 {
				lastCandidate := shots[i].Candidates[len(shots[i].Candidates)-1]
				if lastCandidate.CandidateID != "" {
					candidate.CandidateID = lastCandidate.CandidateID
				}
			}
			if candidate.CandidateID == "" {
				candidate.CandidateID = fmt.Sprintf("%s-candidate-%02d", shotID, candidate.AttemptIndex+1)
			}
		}

		candidates = append(candidates, candidate)
		reports = append(reports, report)
	}

	return candidates, reports, nil
}

// parseShotQAReport converts a raw map to a ShotQAReport.
func parseShotQAReport(raw map[string]interface{}) (videomodel.ShotQAReport, error) {
	report := videomodel.ShotQAReport{}

	// Marshal through JSON for clean conversion.
	data, err := json.Marshal(raw)
	if err != nil {
		return report, fmt.Errorf("marshal shot QA report: %w", err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("unmarshal shot QA report: %w", err)
	}

	// Normalize empty values.
	if report.Status == "" {
		if report.Passed || report.HumanApproved {
			report.Status = videomodel.ShotQAPassed
		} else {
			report.Status = videomodel.ShotQAFailed
		}
	}

	return report, nil
}

func sourceTypeForReport(report videomodel.ShotQAReport) string {
	if report.Passed && !report.HumanApproved {
		return "rendered_shot"
	}
	return "rendered_shot"
}
