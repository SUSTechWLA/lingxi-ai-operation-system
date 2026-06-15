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
			}
		}
	}()

	zap.L().Info("Scheduler started (30s fallback scan)")
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
