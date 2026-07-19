package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type PendingShotRegeneration = model.PendingShotRegeneration

type pendingShotRegenerationStore interface {
	FindPendingShotRegenerations(ctx context.Context, limit int) ([]model.PendingShotRegeneration, error)
}

type ShotRegenerationReconciler struct {
	store     pendingShotRegenerationStore
	service   *CreationService
	interval  time.Duration
	batchSize int
}

func NewShotRegenerationReconciler(store pendingShotRegenerationStore, service *CreationService, interval time.Duration, batchSize int) *ShotRegenerationReconciler {
	return &ShotRegenerationReconciler{store: store, service: service, interval: interval, batchSize: batchSize}
}

func (r *ShotRegenerationReconciler) RunOnce(ctx context.Context) error {
	if r == nil || r.store == nil || r.service == nil || r.batchSize <= 0 {
		return nil
	}
	pending, err := r.store.FindPendingShotRegenerations(ctx, r.batchSize)
	if err != nil {
		return err
	}
	var reconciliationErrors []error
	for _, item := range pending {
		if err := r.service.ResumeShotRegeneration(ctx, item.UserID, item.ProjectID, item.TaskID); err != nil {
			reconciliationErrors = append(reconciliationErrors, fmt.Errorf("resume shot regeneration %s: %w", item.TaskID, err))
		}
	}
	return errors.Join(reconciliationErrors...)
}

func (r *ShotRegenerationReconciler) Run(ctx context.Context) {
	if r == nil || r.interval <= 0 || r.batchSize <= 0 {
		return
	}
	if err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
		zap.L().Warn("shot regeneration startup reconciliation failed", zap.Error(err))
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.RunOnce(ctx); err != nil {
				zap.L().Warn("shot regeneration periodic reconciliation failed", zap.Error(err))
			}
		}
	}
}

func (s *CreationService) ResumeShotRegeneration(ctx context.Context, userID, projectID, taskID string) error {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return err
	}
	result, err := regenerationResult(state, taskID)
	if err != nil {
		return err
	}
	_, err = s.dispatchDurableTask(ctx, userID, projectID, result)
	return err
}
