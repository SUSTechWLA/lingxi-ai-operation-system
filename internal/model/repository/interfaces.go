package repository

import (
	"context"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
)

// NodeRepo defines the interface for node data access.
type NodeRepo interface {
	FindByID(ctx context.Context, id string) (*model.Node, error)
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error)
	FindByStatus(ctx context.Context, status model.NodeStatus) ([]*model.Node, error)
	FindChildNodes(ctx context.Context, parentID string) ([]*model.Node, error)
	Save(ctx context.Context, node *model.Node) error
	UpdateStatus(ctx context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error
}

// TaskRepo defines the interface for task data access.
type TaskRepo interface {
	Save(ctx context.Context, task *model.Task) error
	FindByID(ctx context.Context, id string) (*model.Task, error)
	UpdateStatus(ctx context.Context, id string, status model.TaskStatus) error
}

// DependencyRepo defines the interface for node dependency data access.
type DependencyRepo interface {
	Save(ctx context.Context, dep *model.NodeDependency) error
	FindByChildID(ctx context.Context, childID string) ([]*model.NodeDependency, error)
}

// ContextRepo defines the interface for context data access.
type ContextRepo interface {
	Save(ctx context.Context, c *model.Context) error
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Context, error)
	FindLatestSnapshotByNodeID(ctx context.Context, nodeID string) (*model.Context, error)
}

// Compile-time checks that concrete types satisfy interfaces.
var _ NodeRepo = (*NodeRepository)(nil)
var _ TaskRepo = (*TaskRepository)(nil)
var _ DependencyRepo = (*NodeDependencyRepository)(nil)
var _ ContextRepo = (*ContextRepository)(nil)
