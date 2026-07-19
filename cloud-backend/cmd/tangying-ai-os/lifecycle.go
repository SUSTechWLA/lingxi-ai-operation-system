package main

import "context"

type httpShutdowner interface {
	Shutdown(ctx context.Context) error
}

func shutdownHTTPAndWorkers(ctx context.Context, server httpShutdowner, cancelWorkers func(), joinWorkers func()) error {
	shutdownErr := server.Shutdown(ctx)
	cancelWorkers()
	joinWorkers()
	return shutdownErr
}
