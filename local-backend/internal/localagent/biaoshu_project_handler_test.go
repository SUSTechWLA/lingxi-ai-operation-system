package localagent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBiaoshuProjectRoutesCreateGetAndRegisterArtifact(t *testing.T) {
	outputDir := t.TempDir()
	os.Setenv("BIAOSHU_OUTPUT_DIR", outputDir)
	t.Cleanup(func() { os.Unsetenv("BIAOSHU_OUTPUT_DIR") })
	s := NewServer(Config{DataDir: t.TempDir()})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	createBody := bytes.NewBufferString(`{"projectName":"养护","bidFilePath":"E:/yhbs/招标文件/招标文件_converted.docx"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects", createBody)
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Project BiaoshuProjectManifest `json:"project"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	registerBody := bytes.NewBufferString(`{"kind":"BID_ANALYSIS","name":"招标文件解析报告","storageRef":"E:/out/00_招标文件解析报告.md","mimeType":"text/markdown","status":"valid"}`)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects/"+created.Project.ProjectID+"/artifacts", registerBody)
	registerRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusOK {
		t.Fatalf("register status = %d body=%s", registerRec.Code, registerRec.Body.String())
	}
	var updated struct {
		Project BiaoshuProjectManifest `json:"project"`
	}
	if err := json.Unmarshal(registerRec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Project.CurrentStage != StageAnalysisReady {
		t.Fatalf("stage = %s, want %s", updated.Project.CurrentStage, StageAnalysisReady)
	}
}

func TestBiaoshuProjectCollectionGetIsPureRead(t *testing.T) {
	root := t.TempDir()
	s := NewServer(Config{
		DataDir:          root,
		WorkspaceRoot:    t.TempDir(),
		BiaoshuOutputDir: filepath.Join(t.TempDir(), "output"),
	})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local/biaoshu/projects", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(s.biaoshuProjectRoot()); err == nil {
		t.Fatalf("GET /biaoshu/projects must not create %s", s.biaoshuProjectRoot())
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error for %s: %v", s.biaoshuProjectRoot(), err)
	}
}

func TestCreateBiaoshuProjectReportsManifestDirectoryRole(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, "projects")
	if err := os.WriteFile(blockingFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewServer(Config{
		DataDir:          root,
		WorkspaceRoot:    t.TempDir(),
		BiaoshuOutputDir: filepath.Join(t.TempDir(), "output"),
	})

	body := bytes.NewBufferString(`{"projectName":"养护","bidFilePath":"E:/bid/招标文件.docx"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects", body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "biaoshu project manifest dir") {
		t.Fatalf("error should name manifest directory role, got %s", rec.Body.String())
	}
}

func TestBiaoshuProjectCreateDeleteRecreateSameName(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "output")
	s := NewServer(Config{
		DataDir:          filepath.Join(root, "data"),
		WorkspaceRoot:    root,
		BiaoshuOutputDir: outputRoot,
	})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateWritablePaths(); err != nil {
		t.Fatal(err)
	}

	create := func() BiaoshuProjectManifest {
		body := bytes.NewBufferString(`{"projectName":"test-project","bidFilePath":"E:/bid/招标文件.docx"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects", body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Project BiaoshuProjectManifest `json:"project"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Project
	}

	first := create()

	// Delete the project
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/local/biaoshu/projects/"+first.ProjectID, nil)
	deleteRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}

	// Verify manifest is gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/local/biaoshu/projects/"+first.ProjectID, nil)
	getRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("project should be gone, got status=%d", getRec.Code)
	}

	// Re-create with same name must succeed (no 409 conflict)
	second := create()
	if second.ProjectID == first.ProjectID {
		t.Fatalf("recreated project should have a new projectId, got %s == %s", second.ProjectID, first.ProjectID)
	}
	if second.ProjectName != first.ProjectName {
		t.Fatalf("recreated project name = %s, want %s", second.ProjectName, first.ProjectName)
	}
}
