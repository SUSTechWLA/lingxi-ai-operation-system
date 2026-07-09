package localagent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const biaoshuProjectDirName = "biaoshu"
const biaoshuProjectManifestFilename = "project.json"
const biaoshuProjectMirrorFilename = "project.manifest.json"

type BiaoshuProjectCreateRequest struct {
	ProjectName string `json:"projectName"`
	BidFilePath string `json:"bidFilePath"`
	OutputDir   string `json:"outputDir,omitempty"`
}

type BiaoshuArtifactRegisterRequest struct {
	ID         string                 `json:"id,omitempty"`
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	StorageRef string                 `json:"storageRef"`
	MimeType   string                 `json:"mimeType"`
	DependsOn  []string               `json:"dependsOn,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func (s *Server) biaoshuProjectRoot() string {
	return filepath.Join(s.paths.ProjectDir, biaoshuProjectDirName)
}

func (s *Server) biaoshuProjectDir(projectID string) string {
	return filepath.Join(s.biaoshuProjectRoot(), projectID)
}

func (s *Server) biaoshuProjectManifestPath(projectID string) string {
	return filepath.Join(s.biaoshuProjectDir(projectID), biaoshuProjectManifestFilename)
}

func (s *Server) biaoshuOutputRoot() string {
	return s.cfg.BiaoshuOutputDir
}

func normalizeBiaoshuProjectName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("projectName is required and must not be blank")
	}
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "", errors.New("projectName is required and must not be blank")
	}
	name = strings.Join(fields, " ")
	for _, r := range name {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return "", fmt.Errorf("projectName contains invalid character: %q", r)
		}
	}
	return name, nil
}

func (s *Server) findBiaoshuProjectByName(name string) (BiaoshuProjectManifest, bool) {
	manifests, err := s.listBiaoshuProjectManifests()
	if err != nil {
		return BiaoshuProjectManifest{}, false
	}
	lower := strings.ToLower(name)
	for _, m := range manifests {
		if strings.ToLower(m.ProjectName) == lower {
			return m, true
		}
	}
	return BiaoshuProjectManifest{}, false
}

func (s *Server) createBiaoshuProject(req BiaoshuProjectCreateRequest) (BiaoshuProjectManifest, error) {
	projectName, err := normalizeBiaoshuProjectName(req.ProjectName)
	if err != nil {
		return BiaoshuProjectManifest{}, err
	}
	bidFilePath := strings.TrimSpace(req.BidFilePath)
	if bidFilePath == "" {
		return BiaoshuProjectManifest{}, errors.New("bidFilePath is required")
	}
	if existing, ok := s.findBiaoshuProjectByName(projectName); ok {
		return BiaoshuProjectManifest{}, fmt.Errorf("conflict: project %q already exists (projectId=%s)", projectName, existing.ProjectID)
	}
	proposedOutput := strings.TrimSpace(req.OutputDir)
	if proposedOutput == "" {
		proposedOutput = filepath.Join(s.biaoshuOutputRoot(), projectName)
	}
	if _, err := os.Stat(proposedOutput); err == nil {
		if _, statErr := os.Stat(filepath.Join(proposedOutput, biaoshuProjectMirrorFilename)); statErr == nil {
			return BiaoshuProjectManifest{}, fmt.Errorf("conflict: output directory already exists: %s", proposedOutput)
		}
	}
	projectID := newBiaoshuProjectID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	manifest := BiaoshuProjectManifest{
		SchemaVersion: BiaoshuProjectSchemaVersion,
		ProjectID:     projectID,
		ProjectName:   projectName,
		Status:        "CREATED",
		CurrentStage:  StageCreated,
		CreatedAt:     now,
		UpdatedAt:     now,
		OutputDir:     proposedOutput,
		OutputDirName: projectName,
		SourceFiles: []BiaoshuProjectSourceFile{{
			ID:           "source_1",
			Path:         bidFilePath,
			OriginalName: filepath.Base(bidFilePath),
			AddedAt:      now,
		}},
		Artifacts:   []BiaoshuProjectArtifact{},
		Runs:        []BiaoshuProjectRun{},
		StageEvents: []BiaoshuProjectStageEvent{},
	}
	manifest.StageEvents = append(manifest.StageEvents, BiaoshuProjectStageEvent{
		ID:      "event_" + projectID,
		Type:    "project_created",
		At:      now,
		Message: "项目已创建",
	})
	if err := s.writeBiaoshuProjectManifest(manifest); err != nil {
		return BiaoshuProjectManifest{}, err
	}
	return manifest, nil
}

func (s *Server) readBiaoshuProjectManifest(projectID string) (BiaoshuProjectManifest, error) {
	if !isSafePathSegment(projectID) {
		return BiaoshuProjectManifest{}, errors.New("projectId is required and must be a safe path segment")
	}
	data, err := os.ReadFile(s.biaoshuProjectManifestPath(projectID))
	if err != nil {
		return BiaoshuProjectManifest{}, err
	}
	var manifest BiaoshuProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BiaoshuProjectManifest{}, err
	}
	return manifest, validateBiaoshuProjectManifest(manifest)
}

func (s *Server) writeBiaoshuProjectManifest(manifest BiaoshuProjectManifest) error {
	manifest.CurrentStage = biaoshuStageFromArtifacts(manifest.Artifacts, manifest.Status)
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := validateBiaoshuProjectManifest(manifest); err != nil {
		return err
	}
	path := s.biaoshuProjectManifestPath(manifest.ProjectID)
	manifestDir := filepath.Dir(path)
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return fmt.Errorf("biaoshu project manifest dir is not creatable (%s): %w", manifestDir, err)
	}
	if err := writeIndentedJSON(path, manifest); err != nil {
		return fmt.Errorf("write biaoshu project manifest %s: %w", path, err)
	}
	if err := s.writeBiaoshuProjectManifestMirror(manifest); err != nil {
		return fmt.Errorf("write biaoshu output manifest mirror: %w", err)
	}
	return nil
}

func (s *Server) writeBiaoshuProjectManifestMirror(manifest BiaoshuProjectManifest) error {
	if strings.TrimSpace(manifest.OutputDir) == "" {
		return nil
	}
	if err := os.MkdirAll(manifest.OutputDir, 0o755); err != nil {
		return fmt.Errorf("biaoshu output dir is not creatable (%s): %w", manifest.OutputDir, err)
	}
	return writeIndentedJSON(filepath.Join(manifest.OutputDir, biaoshuProjectMirrorFilename), manifest)
}

func (s *Server) registerBiaoshuProjectArtifact(projectID string, req BiaoshuArtifactRegisterRequest) (BiaoshuProjectManifest, error) {
	manifest, err := s.readBiaoshuProjectManifest(projectID)
	if err != nil {
		return BiaoshuProjectManifest{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	artifactID := strings.TrimSpace(req.ID)
	if artifactID == "" {
		artifactID = "artifact_" + strings.ToLower(string(req.Kind))
	}
	artifact := BiaoshuProjectArtifact{
		ID:         artifactID,
		Kind:       BiaoshuArtifactKind(req.Kind),
		Name:       strings.TrimSpace(req.Name),
		Status:     strings.TrimSpace(req.Status),
		StorageRef: strings.TrimSpace(req.StorageRef),
		MimeType:   strings.TrimSpace(req.MimeType),
		DependsOn:  req.DependsOn,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   req.Metadata,
	}
	if artifact.Name == "" {
		artifact.Name = string(artifact.Kind)
	}
	if artifact.Status == "" {
		artifact.Status = "valid"
	}
	if artifact.MimeType == "" {
		artifact.MimeType = "text/markdown"
	}
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]interface{}{}
	}
	replaced := false
	for i, existing := range manifest.Artifacts {
		if existing.ID == artifact.ID {
			artifact.CreatedAt = existing.CreatedAt
			manifest.Artifacts[i] = artifact
			replaced = true
			break
		}
	}
	if !replaced {
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}
	manifest.StageEvents = append(manifest.StageEvents, BiaoshuProjectStageEvent{
		ID:         fmt.Sprintf("event_%d", time.Now().UTC().UnixNano()),
		Type:       "artifact_registered",
		At:         now,
		Message:    "产物已登记: " + artifact.Name,
		ArtifactID: artifact.ID,
	})
	if artifact.Status == "valid" {
		manifest.Status = "SUCCESS"
	}
	if err := s.writeBiaoshuProjectManifest(manifest); err != nil {
		return BiaoshuProjectManifest{}, err
	}
	return s.readBiaoshuProjectManifest(projectID)
}

func (s *Server) listBiaoshuProjectManifests() ([]BiaoshuProjectManifest, error) {
	root := s.biaoshuProjectRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []BiaoshuProjectManifest{}, nil
		}
		return nil, err
	}
	manifests := []BiaoshuProjectManifest{}
	for _, entry := range entries {
		if !entry.IsDir() || !isSafePathSegment(entry.Name()) {
			continue
		}
		manifest, err := s.readBiaoshuProjectManifest(entry.Name())
		if err != nil {
			continue
		}
		// Only include projects whose output directory still exists.
		if manifest.OutputDir != "" {
			if _, statErr := os.Stat(manifest.OutputDir); os.IsNotExist(statErr) {
				continue
			}
		}
		manifests = append(manifests, manifest)
	}
	sort.SliceStable(manifests, func(i, j int) bool {
		return manifests[i].UpdatedAt > manifests[j].UpdatedAt
	})
	return manifests, nil
}

func biaoshuManagedProjectScore(manifest BiaoshuProjectManifest) int {
	stageWeight := map[BiaoshuProjectStage]int{
		StageWordExported:   90,
		StageDraftMerged:    80,
		StageWordcheckReady: 70,
		StageChaptersReady:  60,
		StageOutlineReady:   50,
		StageScoringReady:   45,
		StageContextReady:   40,
		StageAnalysisReady:  30,
		StageRawParsed:      20,
		StageCreated:        10,
		StageFailed:         0,
	}
	score := stageWeight[manifest.CurrentStage]
	for _, artifact := range manifest.Artifacts {
		if artifact.Status == "valid" && strings.TrimSpace(artifact.StorageRef) != "" {
			score++
		}
	}
	return score
}

func shouldReplaceBiaoshuManagedProject(existing, incoming BiaoshuProjectManifest) bool {
	existingScore := biaoshuManagedProjectScore(existing)
	incomingScore := biaoshuManagedProjectScore(incoming)
	if incomingScore != existingScore {
		return incomingScore > existingScore
	}
	existingTime, existingErr := time.Parse(time.RFC3339Nano, existing.UpdatedAt)
	incomingTime, incomingErr := time.Parse(time.RFC3339Nano, incoming.UpdatedAt)
	if existingErr != nil || incomingErr != nil {
		return false
	}
	return incomingTime.After(existingTime)
}

func (s *Server) importManagedBiaoshuProjectsFromRoots(roots []string) (int, error) {
	imported := 0
	for _, root := range roots {
		projectRoot := filepath.Join(root, "projects", biaoshuProjectDirName)
		entries, err := os.ReadDir(projectRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return imported, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !isSafePathSegment(entry.Name()) {
				continue
			}
			legacyPath := filepath.Join(projectRoot, entry.Name(), biaoshuProjectManifestFilename)
			data, err := os.ReadFile(legacyPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return imported, err
			}
			var legacy BiaoshuProjectManifest
			if err := json.Unmarshal(data, &legacy); err != nil {
				continue
			}
			if err := validateBiaoshuProjectManifest(legacy); err != nil {
				continue
			}

			// Skip manifests whose output directory no longer exists.
			if legacy.OutputDir != "" {
				if _, statErr := os.Stat(legacy.OutputDir); os.IsNotExist(statErr) {
					continue
				}
			}

			current, err := s.readBiaoshuProjectManifest(legacy.ProjectID)
			if err == nil && !shouldReplaceBiaoshuManagedProject(current, legacy) {
				continue
			}

			if err := s.writeBiaoshuProjectManifest(legacy); err != nil {
				return imported, err
			}
			imported++
		}
	}
	return imported, nil
}

func newBiaoshuProjectID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("bp_%d", time.Now().UTC().UnixNano())
	}
	return "bp_" + hex.EncodeToString(b[:])
}

func (s *Server) migrateLegacyBiaoshuProjects() (int, error) {
	legacy, err := s.readBiaoshuProjects()
	if err != nil {
		return 0, err
	}
	existing, err := s.listBiaoshuProjectManifests()
	if err != nil {
		return 0, err
	}
	seenRuns := map[string]bool{}
	for _, project := range existing {
		for _, run := range project.Runs {
			seenRuns[run.RunID] = true
		}
	}
	created := 0
	for _, item := range legacy {
		if seenRuns[item.RunID] {
			continue
		}
		merged := false
		for _, project := range existing {
			if sameBiaoshuProjectSource(project, item) {
				if !biaoshuProjectHasRun(project, item.RunID) {
					project.Runs = append(project.Runs, BiaoshuProjectRun{
						RunID:          item.RunID,
						Status:         item.Status,
						StartedAt:      item.CreatedAt,
						EndedAt:        item.UpdatedAt,
						CloudAvailable: false,
					})
					if err := s.writeBiaoshuProjectManifest(project); err != nil {
						return created, err
					}
				}
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		manifest, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
			ProjectName: item.ProjectName,
			BidFilePath: item.BidFilePath,
		})
		if err != nil {
			return created, err
		}
		manifest.Status = item.Status
		manifest.CreatedAt = item.CreatedAt
		manifest.UpdatedAt = item.UpdatedAt
		manifest.Runs = append(manifest.Runs, BiaoshuProjectRun{
			RunID:          item.RunID,
			Status:         item.Status,
			StartedAt:      item.CreatedAt,
			EndedAt:        item.UpdatedAt,
			CloudAvailable: false,
		})
		if err := s.writeBiaoshuProjectManifest(manifest); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func sameBiaoshuProjectSource(manifest BiaoshuProjectManifest, legacy BiaoshuProjectRecord) bool {
	if strings.TrimSpace(manifest.ProjectName) != strings.TrimSpace(legacy.ProjectName) {
		return false
	}
	if len(manifest.SourceFiles) == 0 {
		return false
	}
	return filepath.Clean(manifest.SourceFiles[0].Path) == filepath.Clean(legacy.BidFilePath)
}

func biaoshuProjectHasRun(manifest BiaoshuProjectManifest, runID string) bool {
	for _, run := range manifest.Runs {
		if run.RunID == runID {
			return true
		}
	}
	return false
}

func (s *Server) deleteBiaoshuProject(projectID string) (map[string]interface{}, error) {
	manifest, err := s.readBiaoshuProjectManifest(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	outputRoot := s.biaoshuOutputRoot()
	deletedPaths := []string{}

	// 1. Delete internal manifest directory
	manifestDir := s.biaoshuProjectDir(projectID)
	if strings.HasPrefix(filepath.Clean(manifestDir), filepath.Clean(s.biaoshuProjectRoot())) {
		manifestFile := s.biaoshuProjectManifestPath(projectID)
		// Remove the manifest file explicitly first (Windows may keep the dir)
		if err := os.Remove(manifestFile); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove manifest file: %w", err)
		}
		// Best-effort remove the directory
		os.RemoveAll(manifestDir)
		deletedPaths = append(deletedPaths, manifestDir)
	}

	// 2. Delete output directory (must be under trusted output root)
	outputDir := filepath.Clean(manifest.OutputDir)
	trustedRoot := filepath.Clean(outputRoot)
	if !strings.HasPrefix(outputDir, trustedRoot+string(filepath.Separator)) && outputDir != trustedRoot {
		return nil, fmt.Errorf("outputDir %s is not within trusted output root %s", outputDir, trustedRoot)
	}
	// Remove mirror file explicitly, then best-effort remove the directory
	mirrorPath := filepath.Join(outputDir, biaoshuProjectMirrorFilename)
	if err := os.Remove(mirrorPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove output mirror: %w", err)
	}
	os.RemoveAll(outputDir)
	if _, statErr := os.Stat(mirrorPath); os.IsNotExist(statErr) {
		deletedPaths = append(deletedPaths, outputDir)
	}

	// 3. Delete local artifacts directory
	artifactDir := filepath.Join(s.paths.ArtifactDir, projectID)
	if _, err := os.Stat(artifactDir); err == nil {
		if err := os.RemoveAll(artifactDir); err != nil {
			return nil, fmt.Errorf("failed to remove artifact dir: %w", err)
		}
		deletedPaths = append(deletedPaths, artifactDir)
	}

	// 4. Delete local cache directory
	cacheDir := filepath.Join(s.paths.CacheDir, projectID)
	if _, err := os.Stat(cacheDir); err == nil {
		if err := os.RemoveAll(cacheDir); err != nil {
			return nil, fmt.Errorf("failed to remove cache dir: %w", err)
		}
		deletedPaths = append(deletedPaths, cacheDir)
	}

	// 5. Delete conversations for runs under this project
	for _, run := range manifest.Runs {
		convPath := filepath.Join(s.paths.ConfigDir, "biaoshu-conversations", run.RunID+".json")
		if _, err := os.Stat(convPath); err == nil {
			if err := os.Remove(convPath); err == nil {
				deletedPaths = append(deletedPaths, convPath)
			}
		}
	}

	// 6. Remove from legacy run index
	if err := s.removeBiaoshuProjectFromLegacyIndex(manifest); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status":       "deleted",
		"projectId":    projectID,
		"projectName":  manifest.ProjectName,
		"deletedPaths": deletedPaths,
	}, nil
}

func (s *Server) removeBiaoshuProjectFromLegacyIndex(manifest BiaoshuProjectManifest) error {
	legacyList, err := s.readBiaoshuProjects()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	filtered := make([]BiaoshuProjectRecord, 0, len(legacyList))
	for i := range legacyList {
		keep := true
		for _, run := range manifest.Runs {
			if legacyList[i].RunID == run.RunID {
				keep = false
				break
			}
		}
		if sameBiaoshuProjectSource(manifest, legacyList[i]) {
			keep = false
		}
		if keep {
			filtered = append(filtered, legacyList[i])
		}
	}
	return s.writeBiaoshuProjects(filtered)
}

func (s *Server) readBiaoshuProjectArtifactContent(projectID, artifactID string) (map[string]interface{}, error) {
	manifest, err := s.readBiaoshuProjectManifest(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	outputDir := filepath.Clean(manifest.OutputDir)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project output directory does not exist: %s", outputDir)
	}
	var artifact *BiaoshuProjectArtifact
	for i := range manifest.Artifacts {
		if manifest.Artifacts[i].ID == artifactID {
			artifact = &manifest.Artifacts[i]
			break
		}
	}
	if artifact == nil {
		return nil, fmt.Errorf("artifact %q not found in project %s", artifactID, projectID)
	}
	filePath := artifact.StorageRef
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(outputDir, filePath)
	}
	filePath = filepath.Clean(filePath)
	if !strings.HasPrefix(filePath, outputDir+string(filepath.Separator)) && filePath != outputDir {
		return nil, fmt.Errorf("artifact storageRef is outside project outputDir")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("artifact file not found: %s", filePath)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("artifact path is a directory: %s", filePath)
	}
	const maxPreview = 2 * 1024 * 1024
	if info.Size() > maxPreview {
		return nil, fmt.Errorf("artifact file too large for preview: %d bytes (max %d)", info.Size(), maxPreview)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact file: %w", err)
	}
	return map[string]interface{}{
		"filePath": filePath,
		"format":   artifact.MimeType,
		"content":  string(data),
		"size":     info.Size(),
	}, nil
}

func (s *Server) writeBiaoshuProjectArtifactContent(projectID, artifactID, content, expectedPrevious string) (map[string]interface{}, error) {
	manifest, err := s.readBiaoshuProjectManifest(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	outputDir := filepath.Clean(manifest.OutputDir)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project output directory does not exist: %s", outputDir)
	}
	var artifact *BiaoshuProjectArtifact
	for i := range manifest.Artifacts {
		if manifest.Artifacts[i].ID == artifactID {
			artifact = &manifest.Artifacts[i]
			break
		}
	}
	if artifact == nil {
		return nil, fmt.Errorf("artifact %q not found in project %s", artifactID, projectID)
	}
	filePath := artifact.StorageRef
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(outputDir, filePath)
	}
	filePath = filepath.Clean(filePath)
	if !strings.HasPrefix(filePath, outputDir+string(filepath.Separator)) && filePath != outputDir {
		return nil, fmt.Errorf("artifact storageRef is outside project outputDir")
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".md" && ext != ".txt" && ext != ".json" {
		return nil, fmt.Errorf("only .md, .txt, and .json files can be written")
	}
	if expectedPrevious != "" {
		current, readErr := os.ReadFile(filePath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return nil, fmt.Errorf("artifact file not found: %s", filePath)
			}
			return nil, fmt.Errorf("failed to read current file: %w", readErr)
		}
		if string(current) != expectedPrevious {
			return nil, fmt.Errorf("file has been modified since last read; please re-open and try again")
		}
	}
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	return map[string]interface{}{
		"filePath": filePath,
		"written":  true,
	}, nil
}
