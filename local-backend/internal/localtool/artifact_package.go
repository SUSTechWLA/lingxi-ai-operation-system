package localtool

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArtifactPackageExecutor packages local project artifacts into a zip archive.
type ArtifactPackageExecutor struct {
	guard *PathGuard
}

// NewArtifactPackageExecutor creates a new artifact packaging executor.
func NewArtifactPackageExecutor(dataDir string) *ArtifactPackageExecutor {
	return &ArtifactPackageExecutor{guard: NewPathGuard(dataDir)}
}

// Execute collects the listed files and writes them into a zip archive.
func (e *ArtifactPackageExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "projectId")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("invalid project id: %w", err)
	}

	// Collect input files
	var includeURIs []string
	if raw, ok := job.Payload["include"].([]interface{}); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok {
				includeURIs = append(includeURIs, s)
			}
		}
	}
	if len(includeURIs) == 0 {
		return nil, fmt.Errorf("include list is required for artifact package")
	}

	// Resolve output path
	outputURI := stringFromPayload(job.Payload, "output")
	if outputURI == "" {
		outputURI = "local://projects/" + projectID + "/packages/project_package.zip"
	}
	outputPath, err := e.guard.ResolveLocalURI(outputURI)
	if err != nil {
		return nil, fmt.Errorf("invalid output path: %w", err)
	}
	if err := e.guard.EnsureWritable(outputURI); err != nil {
		return nil, fmt.Errorf("output path not writable: %w", err)
	}

	// Create the zip file
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("cannot create package file: %w", err)
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	var packagedFiles []string
	for _, uri := range includeURIs {
		// Check context cancellation between entries
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resolved, err := e.guard.ResolveLocalURI(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid include path %s: %w", uri, err)
		}
		if err := e.guard.EnsureReadable(uri); err != nil {
			return nil, fmt.Errorf("include path not readable %s: %w", uri, err)
		}

		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("cannot stat %s: %w", uri, err)
		}

		if info.IsDir() {
			// Walk directory and add all files
			err = filepath.Walk(resolved, func(path string, fi os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if fi.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(filepath.Dir(resolved), path)
				zipName := filepath.Base(resolved) + "/" + filepath.ToSlash(rel)
				if err := addFileToPackage(zw, zipName, path); err != nil {
					return err
				}
				packagedFiles = append(packagedFiles, uri+"/"+filepath.ToSlash(rel))
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to package directory %s: %w", uri, err)
			}
		} else {
			zipName := filepath.Base(resolved)
			if err := addFileToPackage(zw, zipName, resolved); err != nil {
				return nil, fmt.Errorf("failed to package file %s: %w", uri, err)
			}
			packagedFiles = append(packagedFiles, uri)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip: %w", err)
	}

	// Get final file info
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat output package: %w", err)
	}

	localRef := "local://projects/" + projectID + "/packages/project_package.zip"
	packagedAt := time.Now().UTC().Format(time.RFC3339)
	return &Result{Output: map[string]interface{}{
		"success":    true,
		"summary":    "项目产物打包完成",
		"outputRef":  localRef,
		"sizeBytes":  info.Size(),
		"fileCount":  len(packagedFiles),
		"files":      packagedFiles,
		"packagedAt": packagedAt,
		"artifacts": []map[string]interface{}{
			{
				"unitId":         "project-package",
				"kind":           "PROJECT_PACKAGE",
				"name":           "project_package.zip",
				"storageType":    "local",
				"storageRef":     localRef,
				"mimeType":       "application/zip",
				"sizeBytes":      info.Size(),
				"status":         "valid",
				"humanApproved":  false,
				"dependsOn":      []string{"VIDEO", "FFMPEG_PROBE_REPORT", "FINAL_REVIEW"},
				"producedByTool": "artifact_packager",
				"producedByRole": "交付制片",
				"metadata": map[string]interface{}{
					"fileCount":  len(packagedFiles),
					"files":      packagedFiles,
					"packagedAt": packagedAt,
				},
			},
		},
	}}, nil
}

func addFileToPackage(zw *zip.Writer, name, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}
