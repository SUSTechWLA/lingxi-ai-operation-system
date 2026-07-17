package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	InputBrief          InputKind = "brief"
	InputMarkdown       InputKind = "markdown_timeline"
	InputUploadedMP4    InputKind = "uploaded_mp4"
	InputReferenceVideo InputKind = "reference_video"

	ArtifactVideoBrief              = "video_brief"
	ArtifactProposalPacket          = "proposal_packet"
	ArtifactScript                  = "script"
	ArtifactShotList                = "shot_list"
	ArtifactVisualPlan              = "visual_plan"
	ArtifactRenderStrategy          = "render_strategy"
	ArtifactAssetManifest           = "asset_manifest"
	ArtifactCompositionSpec         = "video_composition_spec"
	ArtifactRenderReport            = "render_report"
	ArtifactFinalReview             = "final_review"
	ArtifactPublishPackage          = "publish_package"
	DecisionProposalSelection       = "proposal_selection"
	DecisionRenderStrategySelection = "render_strategy_selection"

	EngineHyperFrames   = "hyperframes"
	EngineSeedance      = "seedance"
	EngineHybrid        = "hybrid"
	EngineExistingMedia = "existing_media"

	RenderModeHyperFramesOnly = "hyperframes_only"
	RenderModeSeedanceHeavy   = "seedance_heavy"
	RenderModeHybrid          = "hybrid"
)

type InputKind string

type Manifest struct {
	ID          string          `json:"id" yaml:"id"`
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description" yaml:"description"`
	InputKinds  []InputKind     `json:"inputKinds" yaml:"inputKinds"`
	Keywords    []string        `json:"keywords" yaml:"keywords"`
	Stages      []StageManifest `json:"stages" yaml:"stages"`
}

type StageManifest struct {
	ID             string   `json:"id" yaml:"id"`
	Director       string   `json:"director" yaml:"director"`
	Produces       string   `json:"produces" yaml:"produces"`
	ReviewRequired bool     `json:"reviewRequired" yaml:"reviewRequired"`
	DependsOn      []string `json:"dependsOn,omitempty" yaml:"dependsOn"`
}

type Registry struct {
	pipelines map[string]*Manifest
}

func LoadDirectory(root string) (*Registry, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("pipeline root is required")
	}
	var manifests []*Manifest
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		manifest, loadErr := loadFile(path)
		if loadErr != nil {
			return loadErr
		}
		manifests = append(manifests, manifest)
		return nil
	})
	if err != nil {
		return nil, err
	}
	reg := &Registry{pipelines: make(map[string]*Manifest, len(manifests))}
	for _, manifest := range manifests {
		if _, exists := reg.pipelines[manifest.ID]; exists {
			return nil, fmt.Errorf("duplicate pipeline id %q", manifest.ID)
		}
		reg.pipelines[manifest.ID] = manifest
	}
	return reg, nil
}

func loadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%s: invalid pipeline yaml: %w", path, err)
	}
	if err := validate(&manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &manifest, nil
}

func validate(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("pipeline manifest is required")
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return fmt.Errorf("pipeline id is required")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return fmt.Errorf("pipeline name is required")
	}
	if len(manifest.Stages) == 0 {
		return fmt.Errorf("pipeline %q must declare at least one stage", manifest.ID)
	}
	seen := map[string]bool{}
	for i, stage := range manifest.Stages {
		if strings.TrimSpace(stage.ID) == "" {
			return fmt.Errorf("stage %d id is required", i)
		}
		if seen[stage.ID] {
			return fmt.Errorf("pipeline %q has duplicate stage %q", manifest.ID, stage.ID)
		}
		seen[stage.ID] = true
		if strings.TrimSpace(stage.Director) == "" {
			return fmt.Errorf("stage %q director is required", stage.ID)
		}
		if strings.TrimSpace(stage.Produces) == "" {
			return fmt.Errorf("stage %q produces is required", stage.ID)
		}
	}
	return nil
}

func (r *Registry) Get(id string) (*Manifest, bool) {
	if r == nil {
		return nil, false
	}
	manifest, ok := r.pipelines[id]
	return manifest, ok
}

func (r *Registry) List() []*Manifest {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.pipelines))
	for key := range r.pipelines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]*Manifest, 0, len(keys))
	for _, key := range keys {
		result = append(result, r.pipelines[key])
	}
	return result
}

type SelectionRequest struct {
	Message string
	Inputs  []InputKind
}

type Selection struct {
	PipelineID         string   `json:"pipelineId"`
	PipelineName       string   `json:"pipelineName"`
	Confidence         float64  `json:"confidence"`
	Reason             string   `json:"reason"`
	MatchedKeywords    []string `json:"matchedKeywords"`
	FirstApprovalStage string   `json:"firstApprovalStage"`
}

func (r *Registry) Select(req SelectionRequest) (*Selection, error) {
	if r == nil || len(r.pipelines) == 0 {
		return nil, fmt.Errorf("no video pipelines loaded")
	}
	var best *Manifest
	var bestScore int
	var bestKeywords []string
	for _, manifest := range r.List() {
		score := 0
		if matchesInput(manifest.InputKinds, req.Inputs) {
			score += 3
		}
		matches := matchedKeywords(manifest.Keywords, req.Message)
		score += len(matches) * 2
		if strings.Contains(strings.ToLower(req.Message), "video") || strings.Contains(req.Message, "视频") {
			score++
		}
		if best == nil || score > bestScore {
			best = manifest
			bestScore = score
			bestKeywords = matches
		}
	}
	if best == nil || bestScore == 0 {
		return nil, fmt.Errorf("no pipeline matched request")
	}
	confidence := float64(bestScore) / 8.0
	if confidence > 1 {
		confidence = 1
	}
	return &Selection{
		PipelineID:         best.ID,
		PipelineName:       best.Name,
		Confidence:         confidence,
		Reason:             fmt.Sprintf("matched %s for request input and keywords", best.Name),
		MatchedKeywords:    bestKeywords,
		FirstApprovalStage: firstApprovalStage(best),
	}, nil
}

func matchesInput(accepted, actual []InputKind) bool {
	if len(accepted) == 0 || len(actual) == 0 {
		return false
	}
	set := map[InputKind]bool{}
	for _, input := range accepted {
		set[input] = true
	}
	for _, input := range actual {
		if set[input] {
			return true
		}
	}
	return false
}

func matchedKeywords(keywords []string, message string) []string {
	var matches []string
	lowerMessage := strings.ToLower(message)
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(message, keyword) || strings.Contains(lowerMessage, strings.ToLower(keyword)) {
			matches = append(matches, keyword)
		}
	}
	return matches
}

func firstApprovalStage(manifest *Manifest) string {
	for _, stage := range manifest.Stages {
		if stage.ReviewRequired {
			return stage.ID
		}
	}
	return ""
}
