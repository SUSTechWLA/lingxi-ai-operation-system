package localtool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathGuard resolves local:// URIs to filesystem paths and enforces
// workspace confinement rules. All local executors must use PathGuard
// to resolve input/output paths.
type PathGuard struct {
	dataDir string
}

// NewPathGuard creates a PathGuard scoped to the given data directory.
func NewPathGuard(dataDir string) *PathGuard {
	return &PathGuard{dataDir: dataDir}
}

// allowedPrefixes defines the local:// URI prefixes that PathGuard will resolve.
var allowedPrefixes = map[string]string{
	"local://projects/":  "projects",
	"local://artifacts/": "artifacts",
	"local://cache/":     "cache",
	"local://logs/":      "logs",
}

// forbiddenSegments lists path segments that are always rejected.
var forbiddenSegments = []string{"..", "~"}

// forbiddenPrefixes lists absolute path prefixes that are always rejected.
var forbiddenPrefixes = []string{
	"/etc/",
	"/System/",
	"/private/etc/",
	"/var/root/",
}

// ResolveLocalURI converts a local:// URI to an absolute filesystem path
// within the data directory. Returns an error if the URI refers to a path
// outside the allowed workspace.
func (g *PathGuard) ResolveLocalURI(uri string) (string, error) {
	if uri == "" {
		return "", fmt.Errorf("empty local URI")
	}

	// Handle local:// scheme
	if strings.HasPrefix(uri, "local://") {
		trimmed := strings.TrimPrefix(uri, "local://")
		trimmed = strings.TrimPrefix(trimmed, "/")

		// Validate the segment doesn't contain traversal
		if err := validateLocalSegment(trimmed); err != nil {
			return "", fmt.Errorf("invalid local URI path: %w", err)
		}

		// Check for forbidden segments throughout the path
		parts := strings.Split(trimmed, "/")
		for _, part := range parts {
			for _, forbidden := range forbiddenSegments {
				if part == forbidden {
					return "", fmt.Errorf("path segment %q is forbidden", part)
				}
			}
		}

		// Resolve to absolute path within data directory
		resolved := filepath.Join(g.dataDir, trimmed)
		if err := ensureInside(g.dataDir, resolved); err != nil {
			return "", fmt.Errorf("local URI escapes workspace: %w", err)
		}
		return resolved, nil
	}

	// Non-local:// paths: reject absolute paths outside workspace
	if filepath.IsAbs(uri) {
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(uri, prefix) {
				return "", fmt.Errorf("access to %s is forbidden", uri)
			}
		}
		if err := ensureInside(g.dataDir, uri); err != nil {
			return "", fmt.Errorf("absolute path outside workspace: %w", err)
		}
		return uri, nil
	}

	// Relative paths are resolved relative to dataDir
	resolved := filepath.Join(g.dataDir, uri)
	if err := ensureInside(g.dataDir, resolved); err != nil {
		return "", fmt.Errorf("relative path escapes workspace: %w", err)
	}
	return resolved, nil
}

// EnsureReadable verifies that the resolved path exists and is readable.
func (g *PathGuard) EnsureReadable(uri string) error {
	path, err := g.ResolveLocalURI(uri)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("cannot stat path %s: %w", path, err)
	}
	if info.IsDir() {
		return nil // directories are readable if stat succeeds
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read path %s: %w", path, err)
	}
	f.Close()
	return nil
}

// EnsureWritable verifies that the resolved path (or its parent directory)
// is writable. For paths that don't exist yet, checks the parent directory.
func (g *PathGuard) EnsureWritable(uri string) error {
	path, err := g.ResolveLocalURI(uri)
	if err != nil {
		return err
	}
	// If the path exists, check it directly
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			// Check we can create a temp file in the directory
			testFile := filepath.Join(path, ".pathguard-write-test")
			if err := os.WriteFile(testFile, []byte("ok"), 0o600); err != nil {
				return fmt.Errorf("directory not writable %s: %w", path, err)
			}
			_ = os.Remove(testFile)
			return nil
		}
		// Check we can open for writing
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return fmt.Errorf("file not writable %s: %w", path, err)
		}
		f.Close()
		return nil
	}
	// Path doesn't exist — check parent directory
	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("cannot create parent directory %s: %w", parentDir, err)
	}
	return nil
}

// DataDir returns the workspace root directory.
func (g *PathGuard) DataDir() string {
	return g.dataDir
}

// ensureInside verifies that a resolved path does not escape the root.
func ensureInside(root, path string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("path escapes workspace: %s", path)
	}
	return nil
}
