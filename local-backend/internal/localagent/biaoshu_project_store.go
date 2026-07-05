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

func (s *Server) createBiaoshuProject(req BiaoshuProjectCreateRequest) (BiaoshuProjectManifest, error) {
	projectName := strings.TrimSpace(req.ProjectName)
	bidFilePath := strings.TrimSpace(req.BidFilePath)
	if projectName == "" {
		return BiaoshuProjectManifest{}, errors.New("projectName is required")
	}
	if bidFilePath == "" {
		return BiaoshuProjectManifest{}, errors.New("bidFilePath is required")
	}
	projectID := newBiaoshuProjectID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	outputDir := strings.TrimSpace(req.OutputDir)
	if outputDir == "" {
		outputDir = filepath.Join(s.paths.ArtifactDir, "biaoshu", projectID)
	}
	manifest := BiaoshuProjectManifest{
		SchemaVersion: BiaoshuProjectSchemaVersion,
		ProjectID:     projectID,
		ProjectName:   projectName,
		Status:        "CREATED",
		CurrentStage:  StageCreated,
		CreatedAt:     now,
		UpdatedAt:     now,
		OutputDir:     outputDir,
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeIndentedJSON(path, manifest); err != nil {
		return err
	}
	return s.writeBiaoshuProjectManifestMirror(manifest)
}

func (s *Server) writeBiaoshuProjectManifestMirror(manifest BiaoshuProjectManifest) error {
	if strings.TrimSpace(manifest.OutputDir) == "" {
		return nil
	}
	if err := os.MkdirAll(manifest.OutputDir, 0o755); err != nil {
		return err
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
		if existing.Kind == artifact.Kind {
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
		if err == nil {
			manifests = append(manifests, manifest)
		}
	}
	sort.SliceStable(manifests, func(i, j int) bool {
		return manifests[i].UpdatedAt > manifests[j].UpdatedAt
	})
	return manifests, nil
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
