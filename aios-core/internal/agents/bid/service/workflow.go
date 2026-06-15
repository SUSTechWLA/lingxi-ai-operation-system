package service

import (
	"fmt"

	bidmodel "github.com/tangying-ai/aios-core/internal/agents/bid/model"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

// BuildBidDAG constructs the bid generation DAG from a project's structure.
func BuildBidDAG(project *bidmodel.BidProject) *model.DAGRequest {
	nodes := []model.NodeRequest{}
	edges := []model.Edge{}

	// Node 1: Parse tender document (external tool)
	nodes = append(nodes, model.NodeRequest{
		ID:   "parse_tender",
		Type: "TOOL",
		Name: "doc_parser",
		Input: map[string]interface{}{
			"file_path": project.TenderFilePath,
			"file_type": "pdf",
			"options": map[string]interface{}{
				"extract_tables": true,
				"extract_images": true,
			},
		},
	})

	// Node 2: Plan bid structure via LLM
	nodes = append(nodes, model.NodeRequest{
		ID:   "plan_structure",
		Type: "LLM",
		Name: "llm_api",
		Input: map[string]interface{}{
			"prompt": fmt.Sprintf(
				"Based on the tender analysis: {{parse_tender.output}}, plan a bid document structure for '%s'. "+
					"Output JSON with chapters array, each chapter having id and title.",
				project.Name,
			),
			"max_tokens": 2000,
		},
	})
	edges = append(edges, model.Edge{From: "parse_tender", To: "plan_structure"})

	// Node 3: Human review — approve structure (CONTROL node)
	nodes = append(nodes, model.NodeRequest{
		ID:   "hr_approve_plan",
		Type: "CONTROL",
		Name: "审核-章节规划",
	})
	edges = append(edges, model.Edge{From: "plan_structure", To: "hr_approve_plan"})

	// Nodes 4..N: Chapter generation (parallel, external tool)
	// Default to a standard bid structure if none configured
	chapterCount := getChapterCount(project)
	for i := 0; i < chapterCount; i++ {
		chID := fmt.Sprintf("gen_chapter_%d", i+1)
		nodes = append(nodes, model.NodeRequest{
			ID:   chID,
			Type: "TOOL",
			Name: "chapter_generator",
			Input: map[string]interface{}{
				"chapter":  map[string]interface{}{"index": i + 1},
				"style":    map[string]interface{}{"tone": "formal"},
			},
		})
		edges = append(edges, model.Edge{From: "hr_approve_plan", To: chID})

		// Human review for each chapter (CONTROL node)
		reviewID := fmt.Sprintf("hr_review_%d", i+1)
		nodes = append(nodes, model.NodeRequest{
			ID:   reviewID,
			Type: "CONTROL",
			Name: fmt.Sprintf("审核-第%d章", i+1),
		})
		edges = append(edges, model.Edge{From: chID, To: reviewID})
	}

	// Compliance check (external tool)
	nodes = append(nodes, model.NodeRequest{
		ID:   "compliance_check",
		Type: "TOOL",
		Name: "compliance_checker",
		Input: map[string]interface{}{
			"chapters":   "{{all_chapters.output}}",
			"score_items": "{{parse_tender.output.score_items}}",
		},
	})
	// Depends on ALL chapter reviews being complete
	for i := 0; i < chapterCount; i++ {
		reviewID := fmt.Sprintf("hr_review_%d", i+1)
		edges = append(edges, model.Edge{From: reviewID, To: "compliance_check"})
	}

	// Final human review (CONTROL node)
	nodes = append(nodes, model.NodeRequest{
		ID:   "hr_final",
		Type: "CONTROL",
		Name: "终稿审核",
	})
	edges = append(edges, model.Edge{From: "compliance_check", To: "hr_final"})

	// Export to Word (external tool)
	nodes = append(nodes, model.NodeRequest{
		ID:   "export_doc",
		Type: "TOOL",
		Name: "doc_exporter",
		Input: map[string]interface{}{
			"format": "docx",
			"cover_info": map[string]interface{}{
				"project_name": project.Name,
			},
		},
	})
	edges = append(edges, model.Edge{From: "hr_final", To: "export_doc"})

	return &model.DAGRequest{Nodes: nodes, Edges: edges}
}

func getChapterCount(project *bidmodel.BidProject) int {
	// Parse from project.Structure JSONB if available
	// Default to 5 chapters for a standard bid
	return 5
}
