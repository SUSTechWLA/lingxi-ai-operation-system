package localagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicWritePrivateFileReplacesContentWithPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider-secrets.json")
	if err := os.WriteFile(path, []byte("old secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := atomicWritePrivateFile(path, []byte("new secret\n")); err != nil {
		t.Fatalf("atomicWritePrivateFile returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new secret\n" {
		t.Fatalf("content = %q, want replacement", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	assertNoAtomicTempFiles(t, dir)
}

func TestAtomicWritePrivateFileRenameFailurePreservesOldContentAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider-secrets.json")
	if err := os.WriteFile(path, []byte("old secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	renameFailure := errors.New("injected rename failure")

	err := atomicWritePrivateFileWithRename(path, []byte("new secret\n"), func(_, _ string) error {
		return renameFailure
	})
	if !errors.Is(err, renameFailure) {
		t.Fatalf("error = %v, want injected rename failure", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "old secret\n" {
		t.Fatalf("failed replacement changed old config to %q", data)
	}
	assertNoAtomicTempFiles(t, dir)
}

func assertNoAtomicTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".provider-secrets-") {
			t.Fatalf("temporary provider config was not removed: %s", entry.Name())
		}
	}
}
