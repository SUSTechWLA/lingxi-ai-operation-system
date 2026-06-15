package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data access for workflow templates.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) FindAll(ctx context.Context) ([]*Template, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, category, dag, created_at, updated_at
		 FROM workflow_templates ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*Template
	for rows.Next() {
		var t Template
		var desc, cat *string
		if err := rows.Scan(&t.ID, &t.Name, &desc, &cat, &t.DAG, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if desc != nil {
			t.Description = *desc
		}
		if cat != nil {
			t.Category = *cat
		}
		templates = append(templates, &t)
	}
	return templates, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*Template, error) {
	var t Template
	var desc, cat *string
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, category, dag, created_at, updated_at
		 FROM workflow_templates WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &desc, &cat, &t.DAG, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if desc != nil {
		t.Description = *desc
	}
	if cat != nil {
		t.Category = *cat
	}
	return &t, nil
}

func (r *Repository) Create(ctx context.Context, t *Template) error {
	var desc, cat *string
	if t.Description != "" {
		desc = &t.Description
	}
	if t.Category != "" {
		cat = &t.Category
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO workflow_templates (id, name, description, category, dag, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID, t.Name, desc, cat, t.DAG, t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *Repository) Update(ctx context.Context, t *Template) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_templates SET name=$2, description=$3, category=$4, dag=$5, updated_at=$6
		 WHERE id=$1`,
		t.ID, t.Name, t.Description, t.Category, t.DAG, time.Now())
	return err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM workflow_templates WHERE id=$1`, id)
	return err
}

// ensure json import is used
var _ = json.Marshal
