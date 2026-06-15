package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bidmodel "github.com/tangying-ai/aios-core/internal/bid/model"
)

// BidRepository provides data access for bid projects, chapters, and templates.
type BidRepository struct {
	pool *pgxpool.Pool
}

// NewBidRepository creates a new BidRepository.
func NewBidRepository(pool *pgxpool.Pool) *BidRepository {
	return &BidRepository{pool: pool}
}

// ── Project CRUD ──

// SaveProject inserts or updates a bid project.
func (r *BidRepository) SaveProject(ctx context.Context, p *bidmodel.BidProject) error {
	tenderAnalysis, _ := json.Marshal(p.TenderAnalysis)
	structure, _ := json.Marshal(p.Structure)
	cfg, _ := json.Marshal(p.Config)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO bid_projects (id, user_id, name, status, task_id, template_id, industry,
		 tender_file_path, tender_file_name, tender_analysis, structure, config, progress, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 ON CONFLICT (id) DO UPDATE SET name=$3, status=$4, task_id=$5, structure=$11, config=$12,
		 progress=$13, tender_analysis=$10, updated_at=$15`,
		p.ID, p.UserID, p.Name, string(p.Status), p.TaskID, p.TemplateID, p.Industry,
		p.TenderFilePath, p.TenderFileName, tenderAnalysis, structure, cfg,
		p.Progress, p.CreatedAt, time.Now(),
	)
	return err
}

// FindProjectByID retrieves a bid project by ID.
func (r *BidRepository) FindProjectByID(ctx context.Context, id string) (*bidmodel.BidProject, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, status, task_id, template_id, industry,
		 tender_file_path, tender_file_name, tender_analysis, structure, config, progress, created_at, updated_at
		 FROM bid_projects WHERE id=$1`, id)

	var p bidmodel.BidProject
	var tenderAnalysis, structure, cfg []byte
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.TaskID, &p.TemplateID, &p.Industry,
		&p.TenderFilePath, &p.TenderFileName, &tenderAnalysis, &structure, &cfg,
		&p.Progress, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.TenderAnalysis = tenderAnalysis
	p.Structure = structure
	p.Config = cfg
	return &p, nil
}

// FindProjects lists bid projects with optional status filter and pagination.
func (r *BidRepository) FindProjects(ctx context.Context, status string, userID string, offset, limit int) ([]*bidmodel.BidProject, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		where += fmt.Sprintf(" AND status=$%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if userID != "" {
		where += fmt.Sprintf(" AND user_id=$%d", argIdx)
		args = append(args, userID)
		argIdx++
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM bid_projects %s", where)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Query with pagination
	query := fmt.Sprintf(
		`SELECT id, user_id, name, status, task_id, template_id, industry,
		 tender_file_path, tender_file_name, tender_analysis, structure, config, progress, created_at, updated_at
		 FROM bid_projects %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var projects []*bidmodel.BidProject
	for rows.Next() {
		var p bidmodel.BidProject
		var tenderAnalysis, structure, cfg []byte
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.TaskID, &p.TemplateID, &p.Industry,
			&p.TenderFilePath, &p.TenderFileName, &tenderAnalysis, &structure, &cfg,
			&p.Progress, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		p.TenderAnalysis = tenderAnalysis
		p.Structure = structure
		p.Config = cfg
		projects = append(projects, &p)
	}
	return projects, total, nil
}

// DeleteProject removes a bid project by ID.
func (r *BidRepository) DeleteProject(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM bid_projects WHERE id=$1`, id)
	return err
}

// ── Chapter CRUD ──

// SaveChapter inserts or updates a bid chapter.
func (r *BidRepository) SaveChapter(ctx context.Context, ch *bidmodel.BidChapter) error {
	scoreItems, _ := json.Marshal(ch.ScoreItems)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO bid_chapters (id, project_id, node_id, title, content, status, review_comment, score_items, sort_order, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (id) DO UPDATE SET content=$5, status=$6, review_comment=$7, node_id=$3, updated_at=$11`,
		ch.ID, ch.ProjectID, ch.NodeID, ch.Title, ch.Content, string(ch.Status),
		ch.ReviewComment, scoreItems, ch.SortOrder, ch.CreatedAt, time.Now(),
	)
	return err
}

// FindChaptersByProject retrieves all chapters for a bid project.
func (r *BidRepository) FindChaptersByProject(ctx context.Context, projectID string) ([]*bidmodel.BidChapter, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, node_id, title, content, status, review_comment, score_items, sort_order, created_at, updated_at
		 FROM bid_chapters WHERE project_id=$1 ORDER BY sort_order`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chapters []*bidmodel.BidChapter
	for rows.Next() {
		var ch bidmodel.BidChapter
		var scoreItems []byte
		if err := rows.Scan(&ch.ID, &ch.ProjectID, &ch.NodeID, &ch.Title, &ch.Content,
			&ch.Status, &ch.ReviewComment, &scoreItems, &ch.SortOrder, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		ch.ScoreItems = scoreItems
		chapters = append(chapters, &ch)
	}
	return chapters, nil
}

// FindChapterByID retrieves a single chapter.
func (r *BidRepository) FindChapterByID(ctx context.Context, id string) (*bidmodel.BidChapter, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, project_id, node_id, title, content, status, review_comment, score_items, sort_order, created_at, updated_at
		 FROM bid_chapters WHERE id=$1`, id)

	var ch bidmodel.BidChapter
	var scoreItems []byte
	err := row.Scan(&ch.ID, &ch.ProjectID, &ch.NodeID, &ch.Title, &ch.Content,
		&ch.Status, &ch.ReviewComment, &scoreItems, &ch.SortOrder, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return nil, err
	}
	ch.ScoreItems = scoreItems
	return &ch, nil
}

// ── Template CRUD ──

// FindTemplates lists all bid templates.
func (r *BidRepository) FindTemplates(ctx context.Context) ([]*bidmodel.BidTemplate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, category, industry, structure, workflow_dag, created_at
		 FROM bid_templates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*bidmodel.BidTemplate
	for rows.Next() {
		var t bidmodel.BidTemplate
		var structure, workflowDAG []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.Category, &t.Industry, &structure, &workflowDAG, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Structure = structure
		t.WorkflowDAG = workflowDAG
		templates = append(templates, &t)
	}
	return templates, nil
}
