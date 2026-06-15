package workflow

import (
	"encoding/json"
	"time"
)

// Template is a reusable DAG workflow blueprint.
type Template struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	DAG         json.RawMessage `json:"dag"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ── API types ──

type CreateTemplateRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	DAG         json.RawMessage `json:"dag" binding:"required"`
}

type UpdateTemplateRequest struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	DAG         json.RawMessage `json:"dag,omitempty"`
}

type InstantiateRequest struct {
	Overrides map[string]interface{} `json:"overrides,omitempty"`
}
