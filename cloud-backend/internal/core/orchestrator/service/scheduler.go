package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
)

type Scheduler struct {
	nodeRepo     repository.NodeRepo
	stateService *StateService
	stateMachine *StateMachine
	producer     eventbus.EventPublisher
	interval     time.Duration
	cancel       context.CancelFunc
}

func NewScheduler(
	nodeRepo repository.NodeRepo,
	stateService *StateService,
	producer eventbus.EventPublisher,
) *Scheduler {
	return &Scheduler{
		nodeRepo:     nodeRepo,
		stateService: stateService,
		producer:     producer,
		interval:     30 * time.Second,
	}
}

// SetStateMachine wires the state machine for heartbeat timeout handling.
func (s *Scheduler) SetStateMachine(sm *StateMachine) {
	s.stateMachine = sm
}

func (s *Scheduler) Start(ctx context.Context) {
	schedCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-schedCtx.Done():
				return
			case <-ticker.C:
				s.recoverStaleCreatedNodes(schedCtx)
				s.recoverStaleReadyNodes(schedCtx)
				s.detectHeartbeatTimeout(schedCtx)
			}
		}
	}()

	zap.L().Info("Scheduler started (30s fallback scan)")
}

// detectHeartbeatTimeout scans for long-running nodes whose heartbeat has timed out.
func (s *Scheduler) detectHeartbeatTimeout(ctx context.Context) {
	if s.stateMachine == nil {
		return
	}

	// Default timeout: 5 minutes (300 seconds)
	nodes, err := s.nodeRepo.FindStaleRunningNodes(ctx, 300)
	if err != nil {
		zap.L().Error("Scheduler: failed to find stale running nodes", zap.Error(err))
		return
	}

	for _, node := range nodes {
		zap.L().Warn("Scheduler: heartbeat timeout detected",
			zap.String("nodeId", node.ID),
			zap.String("taskId", node.TaskID),
		)

		if err := s.stateMachine.OnHeartbeatTimeout(ctx, node.ID); err != nil {
			zap.L().Error("Scheduler: failed to handle heartbeat timeout",
				zap.String("nodeId", node.ID),
				zap.Error(err),
			)
		}
	}
}

// recoverStaleReadyNodes republishes READY node events that were lost
// due to Kafka consumer group issues (e.g. consumer not fully connected when
// the initial event was published). Without this, a node stuck in READY
// status would never be picked up by a worker.
func (s *Scheduler) recoverStaleReadyNodes(ctx context.Context) {
	nodes, err := s.nodeRepo.FindByStatus(ctx, model.NodeReady)
	if err != nil {
		zap.L().Error("Scheduler: failed to find READY nodes", zap.Error(err))
		return
	}

	now := time.Now()
	for _, node := range nodes {
		if now.Sub(node.CreatedAt) < 1*time.Minute {
			continue
		}

		zap.L().Warn("Scheduler: recovering stale READY node (stuck >1m, event may have been lost)",
			zap.String("nodeId", node.ID),
			zap.String("taskId", node.TaskID),
		)

		// Republish the ready event so a worker can pick it up
		payload := buildEventPayload(node.Input, node.Name)
		if err := s.producer.Publish(eventbus.TopicNodeReady, node.IdempotencyKey, eventbus.Event{
			TaskID:         node.TaskID,
			NodeID:         node.ID,
			Type:           string(node.Type),
			Payload:        payload,
			TraceID:        node.TaskID + "-" + node.ID,
			IdempotencyKey: node.IdempotencyKey,
		}); err != nil {
			zap.L().Error("Scheduler: failed to republish READY event",
				zap.String("nodeId", node.ID),
				zap.Error(err),
			)
		}
	}
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Fallback scan: only process CREATED nodes that have been stuck for over 1 minute
func (s *Scheduler) recoverStaleCreatedNodes(ctx context.Context) {
	nodes, err := s.nodeRepo.FindByStatus(ctx, model.NodeCreated)
	if err != nil {
		zap.L().Error("Scheduler: failed to find CREATED nodes", zap.Error(err))
		return
	}

	now := time.Now()
	for _, node := range nodes {
		if now.Sub(node.CreatedAt) < 1*time.Minute {
			continue
		}

		zap.L().Warn("Scheduler: recovering stale CREATED node (stuck >1m)",
			zap.String("nodeId", node.ID),
			zap.String("taskId", node.TaskID),
		)

		met, err := s.stateService.CheckDependenciesMet(ctx, node.ID)
		if err != nil {
			continue
		}
		if met {
			if err := s.stateService.InitializeNodeReady(ctx, node); err != nil {
				zap.L().Error("Scheduler: failed to initialize node as ready", zap.Error(err))
			}
		}
	}
}
