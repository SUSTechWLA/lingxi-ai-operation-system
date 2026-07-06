package localagent

import (
	"os"
	"path/filepath"
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

func TestBiaoshuProjectStageFromScoringBreakdownArtifact(t *testing.T) {
	stage := biaoshuStageFromArtifacts([]BiaoshuProjectArtifact{
		{
			Kind:       ArtifactKindBidAnalysis,
			Status:     "valid",
			StorageRef: "E:/out/00_招标文件解析报告.md",
		},
		{
			Kind:       ArtifactKindProjectContext,
			Status:     "valid",
			StorageRef: "E:/out/01_项目背景信息确认表.md",
		},
		{
			Kind:       ArtifactKindScoringBreakdown,
			Status:     "valid",
			StorageRef: "E:/out/02_评分标准拆解表.md",
		},
	}, "SUCCESS")

	if stage != StageScoringReady {
		t.Fatalf("stage = %s, want %s", stage, StageScoringReady)
	}
}

func TestMigrateLegacyBiaoshuProjectsMergesSameSourceProject(t *testing.T) {
	root := t.TempDir()
	s := NewServer(Config{DataDir: root})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	existing, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "养护3",
		BidFilePath: "E:/yhbs/招标文件/招标文件_converted.docx",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.registerBiaoshuProjectArtifact(existing.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindBidAnalysis),
		Name:       "招标文件解析报告",
		Status:     "valid",
		StorageRef: "E:/out/养护3/00_招标文件解析报告.md",
	}); err != nil {
		t.Fatal(err)
	}

	legacy := BiaoshuProjectListResponse{Projects: []BiaoshuProjectRecord{
		{
			RunID:       "agent_run_legacy",
			ProjectName: "养护3",
			BidFilePath: "E:/yhbs/招标文件/招标文件_converted.docx",
			Status:      "SUCCESS",
			CreatedAt:   "2026-07-06T06:49:50Z",
			UpdatedAt:   "2026-07-06T07:20:16Z",
		},
	}}
	if err := s.writeBiaoshuProjects(legacy.Projects); err != nil {
		t.Fatal(err)
	}

	created, err := s.migrateLegacyBiaoshuProjects()
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("created = %d, want 0", created)
	}

	projects, err := s.listBiaoshuProjectManifests()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("managed projects = %d, want 1", len(projects))
	}
	if len(projects[0].Runs) != 1 || projects[0].Runs[0].RunID != "agent_run_legacy" {
		t.Fatalf("legacy run was not attached to existing project: %+v", projects[0].Runs)
	}
}

func writeLegacyManagedProjectForTest(t *testing.T, root string, manifest BiaoshuProjectManifest) {
	t.Helper()
	path := filepath.Join(root, "projects", "biaoshu", manifest.ProjectID, biaoshuProjectManifestFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSON(path, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestImportLegacyManagedBiaoshuProjectsCopiesTempManifests(t *testing.T) {
	currentRoot := t.TempDir()
	legacyRoot := t.TempDir()
	server := NewServer(Config{DataDir: currentRoot})
	if err := server.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	legacyManifest := BiaoshuProjectManifest{
		SchemaVersion: BiaoshuProjectSchemaVersion,
		ProjectID:     "bp_legacy",
		ProjectName:   "养护3",
		Status:        "SUCCESS",
		CurrentStage:  StageScoringReady,
		CreatedAt:     "2026-07-06T06:49:50Z",
		UpdatedAt:     "2026-07-06T07:20:16Z",
		OutputDir:     filepath.Join(legacyRoot, "artifacts", "biaoshu", "bp_legacy"),
		SourceFiles: []BiaoshuProjectSourceFile{{
			ID:           "source_1",
			Path:         "E:/yhbs/招标文件/招标文件_converted.docx",
			OriginalName: "招标文件_converted.docx",
			AddedAt:      "2026-07-06T06:49:50Z",
		}},
		Artifacts: []BiaoshuProjectArtifact{
			{
				ID:         "artifact_bid_analysis",
				Kind:       ArtifactKindBidAnalysis,
				Name:       "招标文件解析报告",
				Status:     "valid",
				StorageRef: "E:/out/养护3/00_招标文件解析报告.md",
				MimeType:   "text/markdown",
				CreatedAt:  "2026-07-06T06:51:35Z",
				UpdatedAt:  "2026-07-06T06:51:35Z",
				Metadata:   map[string]interface{}{},
			},
			{
				ID:         "artifact_bid_project_context",
				Kind:       ArtifactKindProjectContext,
				Name:       "项目背景信息确认表",
				Status:     "valid",
				StorageRef: "E:/out/养护3/01_项目背景信息确认表.md",
				MimeType:   "text/markdown",
				CreatedAt:  "2026-07-06T07:17:42Z",
				UpdatedAt:  "2026-07-06T07:17:42Z",
				Metadata:   map[string]interface{}{},
			},
			{
				ID:         "artifact_bid_scoring_breakdown",
				Kind:       ArtifactKindScoringBreakdown,
				Name:       "评分标准拆解表",
				Status:     "valid",
				StorageRef: "E:/out/养护3/02_评分标准拆解表.md",
				MimeType:   "text/markdown",
				CreatedAt:  "2026-07-06T07:20:16Z",
				UpdatedAt:  "2026-07-06T07:20:16Z",
				Metadata:   map[string]interface{}{},
			},
		},
		StageEvents: []BiaoshuProjectStageEvent{},
		Runs: []BiaoshuProjectRun{{
			RunID:     "agent_run_legacy",
			Status:    "SUCCESS",
			StartedAt: "2026-07-06T06:49:50Z",
			EndedAt:   "2026-07-06T07:20:16Z",
		}},
	}
	writeLegacyManagedProjectForTest(t, legacyRoot, legacyManifest)

	imported, err := server.importManagedBiaoshuProjectsFromRoots([]string{legacyRoot})
	if err != nil {
		t.Fatal(err)
	}
	if imported != 1 {
		t.Fatalf("imported = %d, want 1", imported)
	}

	projects, err := server.listBiaoshuProjectManifests()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	if projects[0].ProjectID != "bp_legacy" {
		t.Fatalf("projectId = %s, want bp_legacy", projects[0].ProjectID)
	}
	if len(projects[0].Artifacts) != 3 {
		t.Fatalf("artifact count = %d, want 3", len(projects[0].Artifacts))
	}
	if projects[0].CurrentStage != StageScoringReady {
		t.Fatalf("stage = %s, want %s", projects[0].CurrentStage, StageScoringReady)
	}
}

func TestImportLegacyManagedBiaoshuProjectsDoesNotDowngradeCurrentProject(t *testing.T) {
	currentRoot := t.TempDir()
	legacyRoot := t.TempDir()
	server := NewServer(Config{DataDir: currentRoot})
	if err := server.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	current, err := server.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "养护3",
		BidFilePath: "E:/yhbs/招标文件/招标文件_converted.docx",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.registerBiaoshuProjectArtifact(current.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindBidAnalysis),
		Name:       "招标文件解析报告",
		Status:     "valid",
		StorageRef: "E:/out/养护3/00_招标文件解析报告.md",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.registerBiaoshuProjectArtifact(current.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindProjectContext),
		Name:       "项目背景信息确认表",
		Status:     "valid",
		StorageRef: "E:/out/养护3/01_项目背景信息确认表.md",
	}); err != nil {
		t.Fatal(err)
	}

	current, err = server.readBiaoshuProjectManifest(current.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	legacyManifest := current
	legacyManifest.Artifacts = legacyManifest.Artifacts[:1]
	legacyManifest.CurrentStage = StageAnalysisReady
	legacyManifest.UpdatedAt = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	writeLegacyManagedProjectForTest(t, legacyRoot, legacyManifest)

	imported, err := server.importManagedBiaoshuProjectsFromRoots([]string{legacyRoot})
	if err != nil {
		t.Fatal(err)
	}
	if imported != 0 {
		t.Fatalf("imported = %d, want 0 because current project is more complete", imported)
	}

	projects, err := server.listBiaoshuProjectManifests()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	if len(projects[0].Artifacts) != 2 {
		t.Fatalf("artifact count = %d, want 2", len(projects[0].Artifacts))
	}
}
