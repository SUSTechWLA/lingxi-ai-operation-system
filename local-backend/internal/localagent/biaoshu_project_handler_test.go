package localagent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBiaoshuProjectRoutesCreateGetAndRegisterArtifact(t *testing.T) {
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
