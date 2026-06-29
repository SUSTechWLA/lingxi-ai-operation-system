package agentruntime

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ArtifactReviewStatus represents the current state of an artifact review.
type ArtifactReviewStatus string

const (
	ArtifactReviewPending  ArtifactReviewStatus = "PENDING"
	ArtifactReviewApproved ArtifactReviewStatus = "APPROVED"
	ArtifactReviewRejected ArtifactReviewStatus = "REJECTED"
)

// ArtifactReview binds a CONTROL node to the artifacts it is reviewing,
// providing a dedicated review record with reviewer tracking.
type ArtifactReview struct {
	ID            string               `json:"id"`
	TaskID        string               `json:"taskId"`
	NodeID        string               `json:"nodeId"`
	ArtifactID    string               `json:"artifactId,omitempty"`
	StorageRef    string               `json:"storageRef,omitempty"`
	Status        ArtifactReviewStatus `json:"status"`
	ReviewReason  string               `json:"reviewReason,omitempty"`
	ReviewerID    string               `json:"reviewerId,omitempty"`
	ReviewComment string               `json:"reviewComment,omitempty"`
	CreatedAt     time.Time            `json:"createdAt"`
	ReviewedAt    *time.Time           `json:"reviewedAt,omitempty"`
}

// ArtifactReviewStore defines persistence operations for artifact reviews.
type ArtifactReviewStore interface {
	Save(ctx context.Context, review *ArtifactReview) error
	FindByID(ctx context.Context, id string) (*ArtifactReview, error)
	FindByNodeID(ctx context.Context, nodeID string) (*ArtifactReview, error)
	FindByTaskID(ctx context.Context, taskID string) ([]*ArtifactReview, error)
	UpdateStatus(ctx context.Context, id string, status ArtifactReviewStatus, reviewerID, comment string) error
}

// sqlArtifactReviewStore implements ArtifactReviewStore using a *sql.DB.
type sqlArtifactReviewStore struct {
	db *sql.DB
}

// NewSQLArtifactReviewStore creates a new SQL-backed artifact review store.
func NewSQLArtifactReviewStore(db *sql.DB) ArtifactReviewStore {
	return &sqlArtifactReviewStore{db: db}
}

func (s *sqlArtifactReviewStore) Save(ctx context.Context, review *ArtifactReview) error {
	createdAt := review.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	const query = `
		INSERT INTO artifact_reviews (id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			reviewer_id = EXCLUDED.reviewer_id,
			review_comment = EXCLUDED.review_comment,
			reviewed_at = EXCLUDED.reviewed_at
	`
	_, err := s.db.ExecContext(ctx, query,
		review.ID, review.TaskID, review.NodeID,
		review.ArtifactID, review.StorageRef,
		string(review.Status), review.ReviewReason,
		review.ReviewerID, review.ReviewComment,
		createdAt,
	)
	return err
}

func (s *sqlArtifactReviewStore) FindByID(ctx context.Context, id string) (*ArtifactReview, error) {
	const query = `
		SELECT id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at, reviewed_at
		FROM artifact_reviews WHERE id = $1
	`
	row := s.db.QueryRowContext(ctx, query, id)
	return scanArtifactReview(row)
}

func (s *sqlArtifactReviewStore) FindByNodeID(ctx context.Context, nodeID string) (*ArtifactReview, error) {
	const query = `
		SELECT id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at, reviewed_at
		FROM artifact_reviews WHERE node_id = $1 ORDER BY created_at DESC LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query, nodeID)
	return scanArtifactReview(row)
}

func (s *sqlArtifactReviewStore) FindByTaskID(ctx context.Context, taskID string) ([]*ArtifactReview, error) {
	const query = `
		SELECT id, task_id, node_id, artifact_id, storage_ref, status, review_reason, reviewer_id, review_comment, created_at, reviewed_at
		FROM artifact_reviews WHERE task_id = $1 ORDER BY created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []*ArtifactReview
	for rows.Next() {
		r, err := scanArtifactReviewFromRows(rows)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

func (s *sqlArtifactReviewStore) UpdateStatus(ctx context.Context, id string, status ArtifactReviewStatus, reviewerID, comment string) error {
	now := time.Now()
	const query = `
		UPDATE artifact_reviews SET status = $2, reviewer_id = $3, review_comment = $4, reviewed_at = $5
		WHERE id = $1
	`
	result, err := s.db.ExecContext(ctx, query, id, string(status), reviewerID, comment, now)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("artifact review %s not found", id)
	}
	return nil
}

func scanArtifactReview(row *sql.Row) (*ArtifactReview, error) {
	r := &ArtifactReview{}
	var artifactID, storageRef, reviewReason, reviewerID, reviewComment sql.NullString
	var reviewedAt sql.NullTime
	err := row.Scan(&r.ID, &r.TaskID, &r.NodeID, &artifactID, &storageRef,
		&r.Status, &reviewReason, &reviewerID, &reviewComment,
		&r.CreatedAt, &reviewedAt)
	if err != nil {
		return nil, err
	}
	r.ArtifactID = artifactID.String
	r.StorageRef = storageRef.String
	r.ReviewReason = reviewReason.String
	r.ReviewerID = reviewerID.String
	r.ReviewComment = reviewComment.String
	if reviewedAt.Valid {
		r.ReviewedAt = &reviewedAt.Time
	}
	return r, nil
}

func scanArtifactReviewFromRows(rows *sql.Rows) (*ArtifactReview, error) {
	r := &ArtifactReview{}
	var artifactID, storageRef, reviewReason, reviewerID, reviewComment sql.NullString
	var reviewedAt sql.NullTime
	err := rows.Scan(&r.ID, &r.TaskID, &r.NodeID, &artifactID, &storageRef,
		&r.Status, &reviewReason, &reviewerID, &reviewComment,
		&r.CreatedAt, &reviewedAt)
	if err != nil {
		return nil, err
	}
	r.ArtifactID = artifactID.String
	r.StorageRef = storageRef.String
	r.ReviewReason = reviewReason.String
	r.ReviewerID = reviewerID.String
	r.ReviewComment = reviewComment.String
	if reviewedAt.Valid {
		r.ReviewedAt = &reviewedAt.Time
	}
	return r, nil
}
