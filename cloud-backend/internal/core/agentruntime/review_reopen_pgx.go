package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

// pgxReviewReopener owns the creator gate mutation boundary. The gate, its
// dedicated review record, and the audit record are committed together.
type pgxReviewReopener struct{ pool *pgxpool.Pool }

type reviewReopenArtifactIdentity struct {
	ProjectID, WorkflowRunID, TaskID, StageName, ParentID, ProducedByNode string
	IsCurrent                                                             bool
}

func validateReviewReopenArtifact(req ReviewReopenRequest, artifact reviewReopenArtifactIdentity) error {
	validRun := artifact.WorkflowRunID == req.RunID || artifact.WorkflowRunID == req.TaskID
	if artifact.ProjectID != req.ProjectID || !validRun || artifact.TaskID != req.TaskID ||
		artifact.StageName != req.StageName || artifact.ParentID != req.ExpectedArtifactID || !artifact.IsCurrent ||
		strings.TrimSpace(artifact.ProducedByNode) == "" {
		return ErrReviewReferenceMismatch
	}
	return nil
}

func NewPGXReviewReopener(pool *pgxpool.Pool) AtomicReviewReopener {
	if pool == nil {
		return nil
	}
	return &pgxReviewReopener{pool: pool}
}

func (s *pgxReviewReopener) ReopenReviewGateAtomic(ctx context.Context, req ReviewReopenRequest) error {
	if strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.TaskID) == "" || strings.TrimSpace(req.ProjectID) == "" ||
		strings.TrimSpace(req.ReviewID) == "" || strings.TrimSpace(req.NewArtifactID) == "" || strings.TrimSpace(req.AuditID) == "" {
		return ErrReviewReferenceMismatch
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var taskID, nodeType, status, projectID string
	var inputJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT r.task_id, n.type, n.status, COALESCE(n.input, '{}'::jsonb),
		       COALESCE(t.input->>'projectId', t.input->>'projectID',
		         (SELECT wr.project_id FROM workflow_runs wr WHERE wr.task_id=r.task_id ORDER BY wr.created_at DESC LIMIT 1), '')
		FROM agent_runs r
		JOIN ai_node n ON n.task_id=r.task_id AND n.id=$2
		LEFT JOIN ai_task t ON t.id=r.task_id
		WHERE r.id=$1
		FOR UPDATE OF r, n`, req.RunID, req.ReviewID).Scan(&taskID, &nodeType, &status, &inputJSON, &projectID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrReviewNotFound
		}
		return err
	}
	if taskID != req.TaskID || nodeType != string(model.NodeTypeReviewGate) || projectID != req.ProjectID {
		return ErrReviewReferenceMismatch
	}
	var input map[string]interface{}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Errorf("decode review input: %w", err)
	}
	currentArtifact, _ := input["artifactId"].(string)
	stage, _ := input["stage"].(string)
	if req.StageName != "" && stage != req.StageName {
		return ErrReviewReferenceMismatch
	}
	idempotent := status == string(model.NodeReady) && currentArtifact == req.NewArtifactID
	if !idempotent && (req.ExpectedArtifactID == "" || currentArtifact != req.ExpectedArtifactID) {
		return ErrReviewReferenceMismatch
	}
	switch model.NodeStatus(status) {
	case model.NodeReady, model.NodeSuccess, model.NodeFailed:
	default:
		return ErrReviewCannotReopen
	}
	if len(req.SourceNodeRefs) == 0 {
		return ErrReviewReferenceMismatch
	}
	var artifactIdentity reviewReopenArtifactIdentity
	err = tx.QueryRow(ctx, `SELECT project_id,COALESCE(workflow_run_id,''),COALESCE(task_id,''),stage_name,
		COALESCE(parent_id,''),is_current,COALESCE(produced_by_node,'')
		FROM artifacts WHERE id=$1 FOR SHARE`, req.NewArtifactID).Scan(
		&artifactIdentity.ProjectID, &artifactIdentity.WorkflowRunID, &artifactIdentity.TaskID,
		&artifactIdentity.StageName, &artifactIdentity.ParentID, &artifactIdentity.IsCurrent, &artifactIdentity.ProducedByNode,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrReviewReferenceMismatch
		}
		return err
	}
	if err := validateReviewReopenArtifact(req, artifactIdentity); err != nil {
		return err
	}
	var sourceOK bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM ai_node
		WHERE task_id=$1 AND status=$2
		  AND (id=ANY($3::text[]) OR input->>'agentOriginalNodeId'=ANY($3::text[]))
		  AND (id=$4 OR input->>'agentOriginalNodeId'=$4)
	)`, req.TaskID, string(model.NodeSuccess), req.SourceNodeRefs, artifactIdentity.ProducedByNode).Scan(&sourceOK)
	if err != nil {
		return err
	}
	if !sourceOK {
		return ErrReviewReferenceMismatch
	}
	output, _ := json.Marshal(map[string]interface{}{
		"approved": false, "humanApproved": false, "artifactId": req.NewArtifactID, "comment": req.Reason,
	})
	if _, err = tx.Exec(ctx, `UPDATE ai_node SET
		input=COALESCE(input, '{}'::jsonb) || jsonb_build_object('artifactId',$1::text),
		status=$2, output=$3::jsonb, error_message=''
		WHERE id=$4 AND task_id=$5`, req.NewArtifactID, string(model.NodeReady), output, req.ReviewID, req.TaskID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO artifact_reviews
		(id,task_id,node_id,artifact_id,status,review_reason,reviewer_id,review_comment,created_at,reviewed_at)
		VALUES ($1,$2,$1,$3,$4,$5,$6,$5,NOW(),NULL)
		ON CONFLICT (id) DO UPDATE SET artifact_id=EXCLUDED.artifact_id,status=EXCLUDED.status,
		review_reason=EXCLUDED.review_reason,reviewer_id=EXCLUDED.reviewer_id,
		review_comment=EXCLUDED.review_comment,reviewed_at=NULL`,
		req.ReviewID, req.TaskID, req.NewArtifactID, string(ArtifactReviewPending), req.Reason, req.ReviewerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO decision_logs
		(id,workflow_run_id,task_id,stage_name,decision_type,selected,approved_by_user,reviewer_id,comment)
		VALUES ($1,$2,$3,$4,$5,$6,false,$7,$8) ON CONFLICT (id) DO NOTHING`,
		req.AuditID, req.RunID, req.TaskID, req.StageName, DecisionStageEdit, req.NewArtifactID, req.ReviewerID, req.Reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
