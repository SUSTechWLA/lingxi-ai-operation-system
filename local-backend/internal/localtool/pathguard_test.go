package localtool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathGuardResolveLocalURI(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)

	tests := []struct {
		name      string
		uri       string
		wantErr   bool
		wantSuffix string
	}{
		{
			name:      "valid project path",
			uri:       "local://projects/project_001/hyperframes",
			wantSuffix: "projects/project_001/hyperframes",
		},
		{
			name:      "valid artifact path",
			uri:       "local://artifacts/project_001/final.mp4",
			wantSuffix: "artifacts/project_001/final.mp4",
		},
		{
			name:      "valid cache path",
			uri:       "local://cache/project_001/temp",
			wantSuffix: "cache/project_001/temp",
		},
		{
			name:      "valid logs path",
			uri:       "local://logs/project_001/run.log",
			wantSuffix: "logs/project_001/run.log",
		},
		{
			name:    "empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "path traversal with double dot",
			uri:     "local://projects/../escape",
			wantErr: true,
		},
		{
			name:    "path traversal with double dot in segment",
			uri:     "local://projects/project_001/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "forbidden tilde segment",
			uri:     "local://projects/~",
			wantErr: true,
		},
		{
			name:    "forbidden segment ..",
			uri:     "local://projects/..",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := guard.ResolveLocalURI(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for URI %q, got path %q", tt.uri, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !filepath.IsAbs(got) {
				t.Fatalf("expected absolute path, got %q", got)
			}
			expected := filepath.Join(root, tt.wantSuffix)
			if got != expected {
				t.Fatalf("expected %q, got %q", expected, got)
			}
		})
	}
}

func TestPathGuardResolveLocalURIEnsuresInside(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)

	// Create a subdirectory to test relative path resolution
	subDir := filepath.Join(root, "projects", "project_001")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Absolute path within workspace should resolve
	absPath := filepath.Join(root, "projects", "project_001", "hyperframes")
	got, err := guard.ResolveLocalURI(absPath)
	if err != nil {
		t.Fatalf("absolute path within workspace should resolve: %v", err)
	}
	if got != absPath {
		t.Fatalf("expected %q, got %q", absPath, got)
	}

	// Absolute path outside workspace should error
	_, err = guard.ResolveLocalURI("/tmp/some-other-path")
	if err == nil {
		t.Fatal("expected error for absolute path outside workspace")
	}
}

func TestPathGuardRejectsForbiddenAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)

	forbiddenPaths := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/proc/1/environ",
		"/sys/kernel",
		"/dev/null",
		"/System/Library",
		"/private/etc/hosts",
		"/var/root/secret",
	}

	for _, path := range forbiddenPaths {
		t.Run("reject_"+filepath.Base(path), func(t *testing.T) {
			_, err := guard.ResolveLocalURI(path)
			if err == nil {
				t.Fatalf("expected error for forbidden absolute path %q", path)
			}
		})
	}
}

func TestPathGuardEnsureReadable(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)

	// Create a file and verify it's readable
	projectDir := filepath.Join(root, "projects", "project_001")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(projectDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Existing file should be readable
	if err := guard.EnsureReadable("local://projects/project_001/test.txt"); err != nil {
		t.Fatalf("expected readable: %v", err)
	}

	// Directory should be readable
	if err := guard.EnsureReadable("local://projects/project_001"); err != nil {
		t.Fatalf("expected directory readable: %v", err)
	}

	// Non-existing path should error
	err := guard.EnsureReadable("local://projects/project_001/nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestPathGuardEnsureWritable(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)

	// Existing file should be writable
	testFile := filepath.Join(root, "projects", "project_001", "test.txt")
	if err := os.MkdirAll(filepath.Dir(testFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := guard.EnsureWritable("local://projects/project_001/test.txt"); err != nil {
		t.Fatalf("expected writable: %v", err)
	}

	// Non-existing path — parent directory should be created and checked
	if err := guard.EnsureWritable("local://projects/project_001/newdir/output.mp4"); err != nil {
		t.Fatalf("expected writable (parent creates): %v", err)
	}

	// Verify the parent directory was created
	expectedParent := filepath.Join(root, "projects", "project_001", "newdir")
	if info, err := os.Stat(expectedParent); err != nil || !info.IsDir() {
		t.Fatalf("expected parent directory to be created at %q", expectedParent)
	}
}

func TestPathGuardDataDir(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)
	if got := guard.DataDir(); got != root {
		t.Fatalf("DataDir() = %q, want %q", got, root)
	}
}

func TestPathGuardRelativePath(t *testing.T) {
	root := t.TempDir()
	guard := NewPathGuard(root)

	// Relative paths should resolve relative to dataDir
	got, err := guard.ResolveLocalURI("projects/test/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(root, "projects", "test", "file.txt")
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestValidateLocalSegment(t *testing.T) {
	tests := []struct {
		segment string
		wantErr bool
	}{
		{"project_001", false},
		{"my-project", false},
		{"project-123", false},
		{"", true},
		{".", true},
		{"..", true},
		{"../escape", true},
		{"path/with/slash", true},
		{"path\\with\\backslash", true},
	}

	for _, tt := range tests {
		t.Run("segment_"+tt.segment, func(t *testing.T) {
			err := validateLocalSegment(tt.segment)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for segment %q", tt.segment)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for segment %q: %v", tt.segment, err)
			}
		})
	}
}

func TestEnsureInside(t *testing.T) {
	root := "/tmp/test-workspace"

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"inside workspace", "/tmp/test-workspace/projects/p1", false},
		{"workspace itself", "/tmp/test-workspace", false},
		{"outside workspace", "/tmp/other/file.txt", true},
		{"parent traversal", "/tmp/test-workspace/../escape", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureInside(root, tt.path)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for path %q", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
