package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/tangying-ai-operation-system/internal/model"
)

type TaskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

func (r *TaskRepository) Save(ctx context.Context, task *model.Task) error {
	input, _ := json.Marshal(task.Input)
	output, _ := json.Marshal(task.Output)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO ai_task (id, user_id, status, input, output, pause_reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (id) DO UPDATE SET status=$3, output=$5, pause_reason=$6`,
		task.ID, task.UserID, string(task.Status), input, output, task.PauseReason, task.CreatedAt,
	)
	return err
}

func (r *TaskRepository) FindByID(ctx context.Context, id string) (*model.Task, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, input, output, pause_reason, created_at FROM ai_task WHERE id=$1`, id,
	)

	var task model.Task
	var input, output []byte
	var userID *string
	var pauseReason *string

	if err := row.Scan(&task.ID, &userID, &task.Status, &input, &output, &pauseReason, &task.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if userID != nil {
		task.UserID = *userID
	}
	if pauseReason != nil {
		task.PauseReason = *pauseReason
	}

	if len(input) > 0 {
		_ = json.Unmarshal(input, &task.Input)
	}
	if len(output) > 0 {
		_ = json.Unmarshal(output, &task.Output)
	}

	return &task, nil
}

func (r *TaskRepository) UpdateStatus(ctx context.Context, id string, status model.TaskStatus) error {
	_, err := r.pool.Exec(ctx, `UPDATE ai_task SET status=$1 WHERE id=$2`, string(status), id)
	return err
}

func (r *TaskRepository) FindRecent(ctx context.Context) (*model.Task, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, input, output, pause_reason, created_at FROM ai_task ORDER BY created_at DESC LIMIT 1`,
	)

	var task model.Task
	var input, output []byte
	var userID *string
	var pauseReason *string

	if err := row.Scan(&task.ID, &userID, &task.Status, &input, &output, &pauseReason, &task.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if userID != nil {
		task.UserID = *userID
	}
	if pauseReason != nil {
		task.PauseReason = *pauseReason
	}

	if len(input) > 0 {
		_ = json.Unmarshal(input, &task.Input)
	}
	if len(output) > 0 {
		_ = json.Unmarshal(output, &task.Output)
	}

	return &task, nil
}

type NodeRepository struct {
	pool *pgxpool.Pool
}

func NewNodeRepository(pool *pgxpool.Pool) *NodeRepository {
	return &NodeRepository{pool: pool}
}

func (r *NodeRepository) Save(ctx context.Context, node *model.Node) error {
	input, _ := json.Marshal(node.Input)
	output, _ := json.Marshal(node.Output)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO ai_node (id, task_id, type, name, status, input, output, error_message, condition,
		 retry_count, max_retry, priority, worker_group, version, idempotency_key, created_at,
		 started_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT (id) DO UPDATE SET
		 	task_id=$2, status=$5, output=$7, error_message=$8, condition=$9, retry_count=$10,
		 	max_retry=$11, priority=$12, worker_group=$13, version=ai_node.version+1,
		 	idempotency_key=$15, started_at=COALESCE($17, ai_node.started_at),
		 	completed_at=COALESCE($18, ai_node.completed_at)`,
		node.ID, node.TaskID, string(node.Type), node.Name, string(node.Status),
		input, output, node.ErrorMessage, node.Condition,
		node.RetryCount, node.MaxRetry, node.Priority, node.WorkerGroup,
		node.Version, node.IdempotencyKey, node.CreatedAt,
		node.StartedAt, node.CompletedAt,
	)
	return err
}

func (r *NodeRepository) FindByID(ctx context.Context, id string) (*model.Node, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, task_id, type, name, status, input, output, error_message, condition,
		        retry_count, max_retry, priority, worker_group, version, idempotency_key, created_at,
		        started_at, completed_at
		 FROM ai_node WHERE id=$1`, id,
	)

	var node model.Node
	var input, output []byte
	var errorMsg *string
	var condition *string

	if err := row.Scan(
		&node.ID, &node.TaskID, &node.Type, &node.Name, &node.Status,
		&input, &output, &errorMsg, &condition,
		&node.RetryCount, &node.MaxRetry, &node.Priority, &node.WorkerGroup,
		&node.Version, &node.IdempotencyKey, &node.CreatedAt,
		&node.StartedAt, &node.CompletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if errorMsg != nil {
		node.ErrorMessage = *errorMsg
	}
	if condition != nil {
		node.Condition = *condition
	}
	if len(input) > 0 {
		_ = json.Unmarshal(input, &node.Input)
	}
	if len(output) > 0 {
		_ = json.Unmarshal(output, &node.Output)
	}

	return &node, nil
}

func (r *NodeRepository) FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, task_id, type, name, status, input, output, error_message, condition,
		        retry_count, max_retry, priority, worker_group, version, idempotency_key, created_at,
		        started_at, completed_at
		 FROM ai_node WHERE task_id=$1
		 ORDER BY started_at NULLS LAST, created_at`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		var node model.Node
		var input, output []byte
		var errorMsg *string
		var condition *string

		if err := rows.Scan(
			&node.ID, &node.TaskID, &node.Type, &node.Name, &node.Status,
			&input, &output, &errorMsg, &condition,
			&node.RetryCount, &node.MaxRetry, &node.Priority, &node.WorkerGroup,
			&node.Version, &node.IdempotencyKey, &node.CreatedAt,
			&node.StartedAt, &node.CompletedAt,
		); err != nil {
			return nil, err
		}

		if errorMsg != nil {
			node.ErrorMessage = *errorMsg
		}
		if condition != nil {
			node.Condition = *condition
		}
		if len(input) > 0 {
			_ = json.Unmarshal(input, &node.Input)
		}
		if len(output) > 0 {
			_ = json.Unmarshal(output, &node.Output)
		}

		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) FindByStatus(ctx context.Context, status model.NodeStatus) ([]*model.Node, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, task_id, type, name, status, input, output, error_message, condition,
		        retry_count, max_retry, priority, worker_group, version, idempotency_key, created_at,
		        started_at, completed_at
		 FROM ai_node WHERE status=$1`, string(status),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		var node model.Node
		var input, output []byte
		var errorMsg *string
		var condition *string

		if err := rows.Scan(
			&node.ID, &node.TaskID, &node.Type, &node.Name, &node.Status,
			&input, &output, &errorMsg, &condition,
			&node.RetryCount, &node.MaxRetry, &node.Priority, &node.WorkerGroup,
			&node.Version, &node.IdempotencyKey, &node.CreatedAt,
			&node.StartedAt, &node.CompletedAt,
		); err != nil {
			return nil, err
		}

		if errorMsg != nil {
			node.ErrorMessage = *errorMsg
		}
		if condition != nil {
			node.Condition = *condition
		}
		if len(input) > 0 {
			_ = json.Unmarshal(input, &node.Input)
		}
		if len(output) > 0 {
			_ = json.Unmarshal(output, &node.Output)
		}

		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) FindChildNodes(ctx context.Context, parentID string) ([]*model.Node, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT n.id, n.task_id, n.type, n.name, n.status, n.input, n.output, n.error_message, n.condition,
		        n.retry_count, n.max_retry, n.priority, n.worker_group, n.version, n.idempotency_key, n.created_at,
		        n.started_at, n.completed_at
		 FROM ai_node n
		 JOIN ai_node_dependency d ON n.id = d.child_node_id
		 WHERE d.parent_node_id=$1`, parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		var node model.Node
		var input, output []byte
		var errorMsg *string
		var condition *string

		if err := rows.Scan(
			&node.ID, &node.TaskID, &node.Type, &node.Name, &node.Status,
			&input, &output, &errorMsg, &condition,
			&node.RetryCount, &node.MaxRetry, &node.Priority, &node.WorkerGroup,
			&node.Version, &node.IdempotencyKey, &node.CreatedAt,
			&node.StartedAt, &node.CompletedAt,
		); err != nil {
			return nil, err
		}

		if errorMsg != nil {
			node.ErrorMessage = *errorMsg
		}
		if condition != nil {
			node.Condition = *condition
		}
		if len(input) > 0 {
			_ = json.Unmarshal(input, &node.Input)
		}
		if len(output) > 0 {
			_ = json.Unmarshal(output, &node.Output)
		}

		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) UpdateStatus(ctx context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error {
	outputJSON, _ := json.Marshal(output)
	_, err := r.pool.Exec(ctx,
		`UPDATE ai_node SET status=$1, output=COALESCE($2::jsonb, output), error_message=$3 WHERE id=$4`,
		string(status), string(outputJSON), errMsg, id,
	)
	return err
}

type NodeDependencyRepository struct {
	pool *pgxpool.Pool
}

func NewNodeDependencyRepository(pool *pgxpool.Pool) *NodeDependencyRepository {
	return &NodeDependencyRepository{pool: pool}
}

func (r *NodeDependencyRepository) Save(ctx context.Context, dep *model.NodeDependency) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO ai_node_dependency (parent_node_id, child_node_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		dep.ParentNodeID, dep.ChildNodeID,
	)
	return err
}

func (r *NodeDependencyRepository) FindByTaskID(ctx context.Context, taskID string) ([]*model.NodeDependency, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT nd.parent_node_id, nd.child_node_id
		 FROM ai_node_dependency nd
		 JOIN ai_node n ON n.id = nd.parent_node_id
		 WHERE n.task_id = $1`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*model.NodeDependency
	for rows.Next() {
		var dep model.NodeDependency
		if err := rows.Scan(&dep.ParentNodeID, &dep.ChildNodeID); err != nil {
			return nil, err
		}
		deps = append(deps, &dep)
	}

	return deps, nil
}

func (r *NodeDependencyRepository) FindByChildID(ctx context.Context, childID string) ([]*model.NodeDependency, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT parent_node_id, child_node_id FROM ai_node_dependency WHERE child_node_id=$1`, childID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*model.NodeDependency
	for rows.Next() {
		var dep model.NodeDependency
		if err := rows.Scan(&dep.ParentNodeID, &dep.ChildNodeID); err != nil {
			return nil, err
		}
		deps = append(deps, &dep)
	}

	return deps, nil
}

type ContextRepository struct {
	pool *pgxpool.Pool
}

func NewContextRepository(pool *pgxpool.Pool) *ContextRepository {
	return &ContextRepository{pool: pool}
}

func (r *ContextRepository) Save(ctx context.Context, c *model.Context) error {
	metadata, _ := json.Marshal(c.Metadata)
	snapshot, _ := json.Marshal(c.SnapshotData)

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	err := r.pool.QueryRow(ctx,
		`INSERT INTO ai_context (context_type, task_id, node_id, metadata, message, snapshot_data, source_module, source_topic, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		string(c.ContextType), c.TaskID, c.NodeID, metadata, c.Message, snapshot, c.SourceModule, c.SourceTopic, c.CreatedAt,
	).Scan(&c.ID)

	return err
}

func (r *ContextRepository) FindByTaskID(ctx context.Context, taskID string) ([]*model.Context, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, context_type, task_id, node_id, metadata, message, snapshot_data, source_module, source_topic, created_at
		 FROM ai_context WHERE task_id=$1 ORDER BY created_at`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contexts []*model.Context
	for rows.Next() {
		var c model.Context
		var metadata, snapshot []byte
		var nodeID *string
		var createdAt *time.Time

		if err := rows.Scan(&c.ID, &c.ContextType, &c.TaskID, &nodeID, &metadata, &c.Message, &snapshot, &c.SourceModule, &c.SourceTopic, &createdAt); err != nil {
			return nil, err
		}

		if createdAt != nil {
			c.CreatedAt = *createdAt
		}

		if nodeID != nil {
			c.NodeID = *nodeID
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &c.Metadata)
		}
		if len(snapshot) > 0 {
			_ = json.Unmarshal(snapshot, &c.SnapshotData)
		}

		contexts = append(contexts, &c)
	}

	return contexts, nil
}

func (r *ContextRepository) FindLatestSnapshotByNodeID(ctx context.Context, nodeID string) (*model.Context, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, context_type, task_id, node_id, metadata, message, snapshot_data, source_module, source_topic, created_at
		 FROM ai_context WHERE node_id=$1 AND snapshot_data IS NOT NULL
		 ORDER BY created_at DESC LIMIT 1`, nodeID,
	)

	var c model.Context
	var metadata, snapshot []byte
	var nodeIDVal *string
	var createdAt *time.Time

	if err := row.Scan(&c.ID, &c.ContextType, &c.TaskID, &nodeIDVal, &metadata, &c.Message, &snapshot, &c.SourceModule, &c.SourceTopic, &createdAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if createdAt != nil {
		c.CreatedAt = *createdAt
	}
	if nodeIDVal != nil {
		c.NodeID = *nodeIDVal
	}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &c.Metadata)
	}
	if len(snapshot) > 0 {
		_ = json.Unmarshal(snapshot, &c.SnapshotData)
	}

	return &c, nil
}

func NewTaskFromMap(input map[string]interface{}) *model.Task {
	now := time.Now()
	task := &model.Task{
		ID:        generateID(),
		Status:    model.TaskCreated,
		Input:     input,
		CreatedAt: now,
	}
	return task
}

func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomHex(4)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = randRead(b)
	return fmt.Sprintf("%x", b)
}

var randRead = func(b []byte) (int, error) {
	for i := range b {
		b[i] = byte(time.Now().UnixNano() % 256)
	}
	return len(b), nil
}
