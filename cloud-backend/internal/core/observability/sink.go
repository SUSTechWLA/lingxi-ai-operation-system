package observability

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

type compositeSink struct {
	sinks              []Sink
	bestEffortFailures atomic.Uint64
}

type bestEffortSink interface {
	BestEffort() bool
}

func NewCompositeSink(sinks ...Sink) Sink {
	filtered := make([]Sink, 0, len(sinks))
	seen := map[Sink]struct{}{}
	for _, sink := range sinks {
		if sink == nil {
			continue
		}
		typ := reflect.TypeOf(sink)
		if typ.Comparable() {
			if _, ok := seen[sink]; ok {
				continue
			}
			seen[sink] = struct{}{}
		}
		filtered = append(filtered, sink)
	}
	return &compositeSink{sinks: filtered}
}
func (s *compositeSink) Write(ctx context.Context, event Event) error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Write(ctx, event); err != nil {
			if policy, ok := sink.(bestEffortSink); ok && policy.BestEffort() {
				s.bestEffortFailures.Add(1)
				continue
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (s *compositeSink) Close(ctx context.Context) error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Close(ctx); err != nil {
			if policy, ok := sink.(bestEffortSink); ok && policy.BestEffort() {
				s.bestEffortFailures.Add(1)
				continue
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (s *compositeSink) BestEffortFailures() uint64 { return s.bestEffortFailures.Load() }

type summaryRepository interface {
	SaveSummary(context.Context, string, Event) error
}
type RepositorySink struct {
	repository       summaryRepository
	skippedOwnerless atomic.Uint64
}

func NewRepositorySink(repository summaryRepository) *RepositorySink {
	return &RepositorySink{repository: repository}
}
func (s *RepositorySink) Write(ctx context.Context, event Event) error {
	owner, ok := trustedcontext.UserID(ctx)
	if !ok {
		s.skippedOwnerless.Add(1)
		return nil
	}
	return s.repository.SaveSummary(ctx, owner, event)
}
func (s *RepositorySink) Close(context.Context) error { return nil }
func (s *RepositorySink) SkippedOwnerless() uint64    { return s.skippedOwnerless.Load() }
