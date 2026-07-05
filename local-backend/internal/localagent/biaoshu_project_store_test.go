package localagent

import (
	"testing"
	"time"
)

func TestBiaoshuProjectStageFromArtifacts(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	manifest := BiaoshuProjectManifest{
		SchemaVersion: "biaoshu.project.v1",
		ProjectID:     "bp_123",
		ProjectName:   "养护",
		Status:        "SUCCESS",
		CreatedAt:     now,
		UpdatedAt:     now,
		Artifacts: []BiaoshuProjectArtifact{
			{ID: "a1", Kind: ArtifactKindBidAnalysis, Status: "valid", StorageRef: "analysis.md"},
			{ID: "a2", Kind: ArtifactKindProjectContext, Status: "valid", StorageRef: "context.md"},
		},
	}

	stage := biaoshuStageFromArtifacts(manifest.Artifacts, manifest.Status)
	if stage != StageContextReady {
		t.Fatalf("stage = %s, want %s", stage, StageContextReady)
	}
}

func TestValidateBiaoshuProjectManifestRejectsMissingIdentity(t *testing.T) {
	err := validateBiaoshuProjectManifest(BiaoshuProjectManifest{
		SchemaVersion: "biaoshu.project.v1",
		ProjectName:   "养护",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBiaoshuProjectStoreCreateAndRegisterArtifact(t *testing.T) {
	dataDir := t.TempDir()
	s := NewServer(Config{DataDir: dataDir})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	manifest, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "养护",
		BidFilePath: `E:\yhbs\招标文件\招标文件_converted.docx`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProjectID == "" {
		t.Fatal("projectId should be generated")
	}
	if manifest.CurrentStage != StageCreated {
		t.Fatalf("stage = %s, want %s", manifest.CurrentStage, StageCreated)
	}

	updated, err := s.registerBiaoshuProjectArtifact(manifest.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindBidAnalysis),
		Name:       "招标文件解析报告",
		StorageRef: manifest.OutputDir + `/00_招标文件解析报告.md`,
		MimeType:   "text/markdown",
		Status:     "valid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentStage != StageAnalysisReady {
		t.Fatalf("stage = %s, want %s", updated.CurrentStage, StageAnalysisReady)
	}
	if len(updated.Artifacts) != 1 {
		t.Fatalf("artifact count = %d, want 1", len(updated.Artifacts))
	}
}

func TestMigrateLegacyBiaoshuProjectsCreatesManifests(t *testing.T) {
	s := NewServer(Config{DataDir: t.TempDir()})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.upsertBiaoshuProject(BiaoshuProjectRecord{
		RunID:       "agent_run_1",
		ProjectName: "养护",
		BidFilePath: "E:/yhbs/招标文件/招标文件_converted.docx",
		Status:      "SUCCESS",
		CreatedAt:   "2026-07-04T07:20:18Z",
		UpdatedAt:   "2026-07-04T08:20:18Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.migrateLegacyBiaoshuProjects()
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	projects, err := s.listBiaoshuProjectManifests()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(projects))
	}
	if projects[0].Runs[0].RunID != "agent_run_1" {
		t.Fatalf("runId = %s, want agent_run_1", projects[0].Runs[0].RunID)
	}
}
