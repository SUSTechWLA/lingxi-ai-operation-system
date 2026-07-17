package agentruntime

// DirectorAdapter wraps an external DirectorRegistry and adapts its
// Director type to agentruntime.StageDirector via duck-typing.
// The inner registry must implement a Get(stageName string) method that
// returns a value whose pointer implements StageDirector.
type DirectorAdapter struct {
	inner interface {
		Get(stageName string) interface {
			Name() string
			StageName() string
			AllowedTools() []string
			ForbiddenTools() []string
			MaxToolCalls() int
			RequiresApproval() bool
		}
	}
}

// NewDirectorAdapter wraps a concrete director registry (e.g. video/director.Registry).
// The inner type must satisfy the anonymous interface.
func NewDirectorAdapter(inner interface {
	Get(stageName string) interface {
		Name() string
		StageName() string
		AllowedTools() []string
		ForbiddenTools() []string
		MaxToolCalls() int
		RequiresApproval() bool
	}
}) *DirectorAdapter {
	return &DirectorAdapter{inner: inner}
}

// Get returns the stage director for the given stage name, or nil if not found.
func (a *DirectorAdapter) Get(stageName string) StageDirector {
	if a == nil || a.inner == nil {
		return nil
	}
	d := a.inner.Get(stageName)
	if d == nil {
		return nil
	}
	return directorWrapper{d}
}

// directorWrapper adapts a concrete director to agentruntime.StageDirector.
type directorWrapper struct {
	inner interface {
		Name() string
		StageName() string
		AllowedTools() []string
		ForbiddenTools() []string
		MaxToolCalls() int
		RequiresApproval() bool
	}
}

func (w directorWrapper) Name() string             { return w.inner.Name() }
func (w directorWrapper) StageName() string         { return w.inner.StageName() }
func (w directorWrapper) AllowedTools() []string    { return w.inner.AllowedTools() }
func (w directorWrapper) ForbiddenTools() []string  { return w.inner.ForbiddenTools() }
func (w directorWrapper) MaxToolCalls() int         { return w.inner.MaxToolCalls() }
func (w directorWrapper) RequiresApproval() bool    { return w.inner.RequiresApproval() }
