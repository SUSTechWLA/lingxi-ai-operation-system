package modelgateway

import "context"

// Provider is the interface that all model providers must implement.
type Provider interface {
	// Name returns the provider identifier (e.g., "openai", "fake").
	Name() string

	// Supports returns true if this provider supports the given capability.
	Supports(cap Capability) bool

	// Execute runs a model request and returns the result.
	Execute(ctx context.Context, req *ModelRequest) (*ModelResult, error)

	// Health checks if the provider is available.
	Health(ctx context.Context) error
}
