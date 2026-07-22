package localagent

import (
	"os"
	"path/filepath"
	"runtime"
)

type renameFileFunc func(oldPath, newPath string) error

func atomicWritePrivateFile(path string, data []byte) error {
	return atomicWritePrivateFileWithRename(path, data, os.Rename)
}

func atomicWritePrivateFileWithRename(path string, data []byte, rename renameFileFunc) (returnErr error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".provider-secrets-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if returnErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	temp = nil
	if err := rename(tempPath, path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
