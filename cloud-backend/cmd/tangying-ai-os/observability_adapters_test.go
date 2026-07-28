package main

import (
	"context"
	"errors"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

type adapterDecisionStore struct{ saved *workflow.DecisionLogRecord }

func (s *adapterDecisionStore) Save(_ context.Context, r *workflow.DecisionLogRecord) error {
	s.saved = r
	return nil
}
func (*adapterDecisionStore) FindByRun(context.Context, string) ([]*workflow.DecisionLogRecord, error) {
	return nil, nil
}
func (*adapterDecisionStore) FindByStage(context.Context, string, string) ([]*workflow.DecisionLogRecord, error) {
	return nil, nil
}

type adapterResolver struct {
	runID string
	err   error
}

func (r adapterResolver) FindRunIDByTaskID(context.Context, string) (string, error) {
	return r.runID, r.err
}

func TestDecisionLogAdapterForwardsClaimedIdentityToInvariantStore(t *testing.T) {
	store := &adapterDecisionStore{}
	adapter := &decisionLogAdapter{store: store}
	if err := adapter.Save(context.Background(), &agentruntime.DecisionLogRecord{TaskID: "task-1", WorkflowRunID: "wfr-claimed"}); err != nil {
		t.Fatal(err)
	}
	if store.saved.WorkflowRunID != "wfr-claimed" {
		t.Fatalf("workflowRunID=%q", store.saved.WorkflowRunID)
	}
}

func TestDecisionLogAdapterRejectsNonemptyRunWithoutTask(t *testing.T) {
	store := &adapterDecisionStore{}
	err := (&decisionLogAdapter{store: store}).Save(context.Background(), &agentruntime.DecisionLogRecord{WorkflowRunID: "agent_run_false"})
	if err == nil || store.saved != nil {
		t.Fatalf("err=%v saved=%v", err, store.saved)
	}
}

type adapterRunService struct {
	called       bool
	runID, stage string
	status       workflow.StageStatus
}

func (s *adapterRunService) UpdateStageStatus(_ context.Context, runID, stage string, status workflow.StageStatus) error {
	s.called = true
	s.runID = runID
	s.stage = stage
	s.status = status
	return nil
}

func TestRunStatusSyncerUsesInstrumentedServiceBoundary(t *testing.T) {
	service := &adapterRunService{}
	syncer := &runStatusSyncer{service: service, resolver: adapterResolver{runID: "wfr-1"}}
	if err := syncer.UpdateStageStatus(context.Background(), "wfr-1", "render", "RUNNING"); err != nil {
		t.Fatal(err)
	}
	if !service.called || service.runID != "wfr-1" || service.stage != "render" || service.status != workflow.StageStatus("RUNNING") {
		t.Fatalf("service=%+v", service)
	}
	if _, err := (&runStatusSyncer{service: service, resolver: adapterResolver{err: errors.New("lookup")}}).FindRunIDByTaskID(context.Background(), "t"); err == nil {
		t.Fatal("expected resolver error")
	}
}
