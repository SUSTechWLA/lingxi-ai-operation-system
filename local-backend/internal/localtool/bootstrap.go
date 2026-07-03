package localtool

// ExecutorConfig holds configuration for all local tool executors.
type ExecutorConfig struct {
	// DataDir is the root directory for all local data (projects, artifacts, cache, logs).
	DataDir string
}

// RegisterDefaultExecutors registers all standard local tool executors on the
// given Registry using the provided configuration. Call this once at startup
// instead of registering each executor individually.
func RegisterDefaultExecutors(reg *Registry, cfg ExecutorConfig) error {
	guard := NewPathGuard(cfg.DataDir)

	reg.Register(
		NewFinalReviewExecutor(guard, cfg.DataDir),
		CommandFinalReview,
	)

	_ = guard

	return nil
}
