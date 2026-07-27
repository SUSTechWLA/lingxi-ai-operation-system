package observability

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

type compositeSink struct{ sinks []Sink }

func NewCompositeSink(sinks ...Sink) Sink {
	filtered := make([]Sink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	return &compositeSink{sinks: filtered}
}
func (s *compositeSink) Write(ctx context.Context, event Event) error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Write(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (s *compositeSink) Close(ctx context.Context) error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

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
