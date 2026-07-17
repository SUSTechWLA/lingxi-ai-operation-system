package agentruntime

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxArtifactReviewStore struct {
	pool *pgxpool.Pool
}

// NewPGXArtifactReviewStore creates an ArtifactReviewStore backed by pgxpool.Pool.
func NewPGXArtifactReviewStore(pool *pgxpool.Pool) ArtifactReviewStore {
	if pool == nil {
		return nil
	}
	return &pgxArtifactReviewStore{pool: pool}
}

func (s *pgxArtifactReviewStore) Save(ctx context.Context, review *ArtifactReview) error {
	createdAt := review.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	const query = `
		INSERT INTO artifact_reviews (id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			artifact_id = EXCLUDED.artifact_id,
			storage_ref = EXCLUDED.storage_ref,
			status = EXCLUDED.status,
			review_reason = EXCLUDED.review_reason,
			reviewer_id = EXCLUDED.reviewer_id,
			review_comment = EXCLUDED.review_comment
	`
	_, err := s.pool.Exec(ctx, query,
		review.ID, review.TaskID, review.NodeID,
		review.ArtifactID, review.StorageRef,
		string(review.Status), review.ReviewReason,
		review.ReviewerID, review.ReviewComment,
		createdAt,
	)
	return err
}

func (s *pgxArtifactReviewStore) FindByID(ctx context.Context, id string) (*ArtifactReview, error) {
	const query = `
		SELECT id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at, reviewed_at
		FROM artifact_reviews WHERE id = $1
	`
	return scanPGXArtifactReview(s.pool.QueryRow(ctx, query, id))
}

func (s *pgxArtifactReviewStore) FindByNodeID(ctx context.Context, nodeID string) (*ArtifactReview, error) {
	const query = `
		SELECT id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at, reviewed_at
		FROM artifact_reviews WHERE node_id = $1 ORDER BY created_at DESC LIMIT 1
	`
	return scanPGXArtifactReview(s.pool.QueryRow(ctx, query, nodeID))
}

func (s *pgxArtifactReviewStore) FindByTaskID(ctx context.Context, taskID string) ([]*ArtifactReview, error) {
	const query = `
		SELECT id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at, reviewed_at
		FROM artifact_reviews WHERE task_id = $1 ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reviews := []*ArtifactReview{}
	for rows.Next() {
		review, err := scanPGXArtifactReview(rows)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}

func (s *pgxArtifactReviewStore) UpdateStatus(ctx context.Context, id string, status ArtifactReviewStatus, reviewerID, comment string) error {
	now := time.Now()
	const query = `
		UPDATE artifact_reviews SET status = $2, reviewer_id = $3, review_comment = $4, reviewed_at = $5
		WHERE id = $1
	`
	tag, err := s.pool.Exec(ctx, query, id, string(status), reviewerID, comment, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("artifact review %s not found", id)
	}
	return nil
}

func scanPGXArtifactReview(row interface {
	Scan(dest ...interface{}) error
}) (*ArtifactReview, error) {
	review := &ArtifactReview{}
	var artifactID, storageRef, reviewReason, reviewerID, reviewComment *string
	var reviewedAt *time.Time
	if err := row.Scan(
		&review.ID,
		&review.TaskID,
		&review.NodeID,
		&artifactID,
		&storageRef,
		&review.Status,
		&reviewReason,
		&reviewerID,
		&reviewComment,
		&review.CreatedAt,
		&reviewedAt,
	); err != nil {
		return nil, err
	}
	review.ArtifactID = stringValue(artifactID)
	review.StorageRef = stringValue(storageRef)
	review.ReviewReason = stringValue(reviewReason)
	review.ReviewerID = stringValue(reviewerID)
	review.ReviewComment = stringValue(reviewComment)
	review.ReviewedAt = reviewedAt
	return review, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ ArtifactReviewStore = (*pgxArtifactReviewStore)(nil)
