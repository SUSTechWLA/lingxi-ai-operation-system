package localagent

import (
	"testing"
)

func historyContainsProjectName(projects []BiaoshuHistoryProject, name string) bool {
	for _, p := range projects {
		if p.ProjectName == name {
			return true
		}
	}
	return false
}

func TestBuildBiaoshuHistoryIncludesLegacyTempManagedProjects(t *testing.T) {
	dataDir := t.TempDir()
	s := NewServer(Config{DataDir: dataDir})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	legacyRoot := t.TempDir()

	legacyManifest := BiaoshuProjectManifest{
		SchemaVersion: BiaoshuProjectSchemaVersion,
		ProjectID:     "bp_yanghu3",
		ProjectName:   "养护3",
		Status:        "SUCCESS",
		CurrentStage:  StageScoringReady,
		CreatedAt:     "2026-07-01T10:00:00Z",
		UpdatedAt:     "2026-07-01T10:54:58Z",
		SourceFiles: []BiaoshuProjectSourceFile{{
			ID:           "source_1",
			Path:         `E:\yhbs\招标文件\招标文件_converted.docx`,
			OriginalName: "招标文件_converted.docx",
			AddedAt:      "2026-07-01T10:00:00Z",
		}},
		OutputDir: `E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护3`,
		Artifacts: []BiaoshuProjectArtifact{
			{
				ID:         "artifact_bid_analysis",
				Kind:       ArtifactKindBidAnalysis,
				Name:       "招标文件解析报告",
				Status:     "valid",
				StorageRef: `E:\out\养护3\00_招标文件解析报告.md`,
				MimeType:   "text/markdown",
				CreatedAt:  "2026-07-01T10:30:00Z",
				UpdatedAt:  "2026-07-01T10:30:00Z",
				Metadata:   map[string]interface{}{},
			},
			{
				ID:         "artifact_bid_scoring_breakdown",
				Kind:       ArtifactKindScoringBreakdown,
				Name:       "评分标准拆解表",
				Status:     "valid",
				StorageRef: `E:\out\养护3\02_评分标准拆解表.md`,
				MimeType:   "text/markdown",
				CreatedAt:  "2026-07-01T10:54:58Z",
				UpdatedAt:  "2026-07-01T10:54:58Z",
				Metadata:   map[string]interface{}{},
			},
		},
		StageEvents: []BiaoshuProjectStageEvent{},
		Runs: []BiaoshuProjectRun{{
			RunID:     "agent_run_yanghu3",
			Status:    "SUCCESS",
			StartedAt: "2026-07-01T10:00:00Z",
			EndedAt:   "2026-07-01T10:54:58Z",
		}},
	}
	writeLegacyManagedProjectForTest(t, legacyRoot, legacyManifest)

	resp, err := s.buildBiaoshuHistory([]string{legacyRoot})
	if err != nil {
		t.Fatalf("buildBiaoshuHistory returned error: %v", err)
	}
	if !historyContainsProjectName(resp.Projects, "养护3") {
		t.Fatalf("expected history to include 养护3, got %#v", resp.Projects)
	}
	if resp.Sources.LegacyTempManaged != 1 {
		t.Fatalf("expected legacy temp managed count 1, got %d", resp.Sources.LegacyTempManaged)
	}
}

func TestBuildBiaoshuHistoryMergesManagedAndLegacyRuns(t *testing.T) {
	dataDir := t.TempDir()
	s := NewServer(Config{DataDir: dataDir})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	legacyRoot := t.TempDir()

	// Create a managed project (养护2) in legacy temp root.
	legacyManifest := BiaoshuProjectManifest{
		SchemaVersion: BiaoshuProjectSchemaVersion,
		ProjectID:     "bp_yanghu2",
		ProjectName:   "养护2",
		Status:        "SUCCESS",
		CurrentStage:  StageContextReady,
		CreatedAt:     "2026-07-01T09:00:00Z",
		UpdatedAt:     "2026-07-01T09:30:00Z",
		SourceFiles: []BiaoshuProjectSourceFile{{
			ID:           "source_1",
			Path:         `E:\yhbs\招标文件\招标文件2_converted.docx`,
			OriginalName: "招标文件2_converted.docx",
			AddedAt:      "2026-07-01T09:00:00Z",
		}},
		OutputDir: `E:\out\养护2`,
		Artifacts: []BiaoshuProjectArtifact{
			{
				ID:         "artifact_bid_analysis",
				Kind:       ArtifactKindBidAnalysis,
				Name:       "招标文件解析报告",
				Status:     "valid",
				StorageRef: `E:\out\养护2\00_招标文件解析报告.md`,
				MimeType:   "text/markdown",
				CreatedAt:  "2026-07-01T09:15:00Z",
				UpdatedAt:  "2026-07-01T09:15:00Z",
				Metadata:   map[string]interface{}{},
			},
		},
		StageEvents: []BiaoshuProjectStageEvent{},
		Runs: []BiaoshuProjectRun{{
			RunID:     "agent_run_yanghu2",
			Status:    "SUCCESS",
			StartedAt: "2026-07-01T09:00:00Z",
			EndedAt:   "2026-07-01T09:30:00Z",
		}},
	}
	writeLegacyManagedProjectForTest(t, legacyRoot, legacyManifest)

	// Create a legacy run project (养护) in the legacy run store.
	if _, _, err := s.upsertBiaoshuProject(BiaoshuProjectRecord{
		RunID:       "agent_run_yanghu1",
		ProjectName: "养护",
		BidFilePath: `E:\yhbs\招标文件\招标文件_converted.docx`,
		Status:      "SUCCESS",
		CreatedAt:   "2026-07-01T08:00:00Z",
		UpdatedAt:   "2026-07-01T08:30:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := s.buildBiaoshuHistory([]string{legacyRoot})
	if err != nil {
		t.Fatalf("buildBiaoshuHistory returned error: %v", err)
	}
	if !historyContainsProjectName(resp.Projects, "养护") {
		t.Fatalf("expected history to include 养护, got %#v", resp.Projects)
	}
	if !historyContainsProjectName(resp.Projects, "养护2") {
		t.Fatalf("expected history to include 养护2, got %#v", resp.Projects)
	}
	if resp.Sources.LegacyTempManaged != 1 {
		t.Fatalf("expected legacy temp managed count 1, got %d", resp.Sources.LegacyTempManaged)
	}
	if resp.Sources.LegacyRuns != 1 {
		t.Fatalf("expected legacy runs count 1, got %d", resp.Sources.LegacyRuns)
	}
}

func TestBuildBiaoshuHistoryDedupSameProjectAcrossSources(t *testing.T) {
	root := t.TempDir()
	s := NewServer(Config{DataDir: root})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	legacyRoot := t.TempDir()

	// Create a managed project in current data dir.
	current, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "养护3",
		BidFilePath: `E:\yhbs\招标文件\招标文件_converted.docx`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.registerBiaoshuProjectArtifact(current.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindBidAnalysis),
		Name:       "招标文件解析报告",
		Status:     "valid",
		StorageRef: `E:\out\养护3\00_招标文件解析报告.md`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.registerBiaoshuProjectArtifact(current.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindProjectContext),
		Name:       "项目背景信息确认表",
		Status:     "valid",
		StorageRef: `E:\out\养护3\01_项目背景信息确认表.md`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.registerBiaoshuProjectArtifact(current.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindScoringBreakdown),
		Name:       "评分标准拆解表",
		Status:     "valid",
		StorageRef: `E:\out\养护3\02_评分标准拆解表.md`,
	}); err != nil {
		t.Fatal(err)
	}

	// Re-read to get updated manifest.
	full, err := s.readBiaoshuProjectManifest(current.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	// Write same project to legacy temp with fewer artifacts (should be deduped).
	stale := full
	stale.Artifacts = stale.Artifacts[:1]
	stale.CurrentStage = StageAnalysisReady
	writeLegacyManagedProjectForTest(t, legacyRoot, stale)

	resp, err := s.buildBiaoshuHistory([]string{legacyRoot})
	if err != nil {
		t.Fatalf("buildBiaoshuHistory returned error: %v", err)
	}
	if !historyContainsProjectName(resp.Projects, "养护3") {
		t.Fatalf("expected history to include 养护3, got %#v", resp.Projects)
	}
	if len(resp.Projects) != 1 {
		t.Fatalf("expected 1 project after dedup, got %d: %#v", len(resp.Projects), resp.Projects)
	}
	// The surviving record should be the more complete one (3 artifacts).
	for _, p := range resp.Projects {
		if p.ProjectName == "养护3" && p.GeneratedCount != 3 {
			t.Fatalf("expected 3 generated artifacts, got %d", p.GeneratedCount)
		}
	}
}
