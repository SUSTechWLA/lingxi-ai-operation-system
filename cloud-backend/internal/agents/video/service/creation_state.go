package service

import (
	"encoding/json"
	"time"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

const shotDrivenStateKey = "shotDrivenState"

func DecodeShotDrivenState(raw json.RawMessage) (model.ShotDrivenState, error) {
	if len(raw) == 0 {
		return emptyShotDrivenState(), nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return model.ShotDrivenState{}, err
	}
	payload, ok := envelope[shotDrivenStateKey]
	if !ok || len(payload) == 0 {
		return emptyShotDrivenState(), nil
	}
	var state model.ShotDrivenState
	if err := json.Unmarshal(payload, &state); err != nil {
		return model.ShotDrivenState{}, err
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	if state.ShotHistory == nil {
		state.ShotHistory = map[string][]model.ShotRevision{}
	}
	if state.RegenerationTasks == nil {
		state.RegenerationTasks = map[string]model.ShotRegenerationTask{}
	}
	if state.IdempotencyTasks == nil {
		state.IdempotencyTasks = map[string]string{}
	}
	if state.ShotMutationReceipts == nil {
		state.ShotMutationReceipts = map[string]model.ShotMutationReceipt{}
	}
	if state.AssemblyReceipts == nil {
		state.AssemblyReceipts = map[string]model.AssemblyReceipt{}
	}
	if state.UpstreamRevisions == nil {
		state.UpstreamRevisions = map[string]time.Time{}
	}
	return state, nil
}

func EncodeShotDrivenState(raw json.RawMessage, state model.ShotDrivenState) (json.RawMessage, error) {
	envelope := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, err
		}
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	state.UpdatedAt = time.Now()
	payload, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	envelope[shotDrivenStateKey] = payload
	out, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

func emptyShotDrivenState() model.ShotDrivenState {
	return model.ShotDrivenState{
		SchemaVersion:        1,
		Shots:                []model.ShotUnit{},
		ShotHistory:          map[string][]model.ShotRevision{},
		RegenerationTasks:    map[string]model.ShotRegenerationTask{},
		IdempotencyTasks:     map[string]string{},
		ShotMutationReceipts: map[string]model.ShotMutationReceipt{},
		AssemblyReceipts:     map[string]model.AssemblyReceipt{},
		UpstreamRevisions:    map[string]time.Time{},
	}
}
