package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
)

type Scheduler struct {
	nodeRepo     *repository.NodeRepository
	stateService *StateService
	producer     *eventbus.Producer
	interval     time.Duration
	cancel       context.CancelFunc
}

func NewScheduler(
	nodeRepo *repository.NodeRepository,
	stateService *StateService,
	producer *eventbus.Producer,
) *Scheduler {
	return &Scheduler{
		nodeRepo:     nodeRepo,
		stateService: stateService,
		producer:     producer,
		interval:     1 * time.Second,
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
				s.recoverCreatedNodes(schedCtx)
			}
		}
	}()

	zap.L().Info("Scheduler started")
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Scheduler) recoverCreatedNodes(ctx context.Context) {
	nodes, err := s.nodeRepo.FindByStatus(ctx, model.NodeCreated)
	if err != nil {
		zap.L().Error("Scheduler: failed to find CREATED nodes", zap.Error(err))
		return
	}

	for _, node := range nodes {
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
