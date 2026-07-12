package localagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBiaoshuBootstrapReturnsManagedProjectsAndResumeProject(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir(), BiaoshuOutputDir: t.TempDir()})
	project, err := server.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "恢复测试项目",
		BidFilePath: `E:\bids\resume.docx`,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local/biaoshu/bootstrap", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var response BiaoshuBootstrapResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "READY" {
		t.Fatalf("status = %q", response.Status)
	}
	if response.ResumeProjectID != project.ProjectID {
		t.Fatalf("resumeProjectId = %q, want %q", response.ResumeProjectID, project.ProjectID)
	}
	if len(response.Projects) != 1 || response.Projects[0].ProjectID != project.ProjectID {
		t.Fatalf("projects = %#v", response.Projects)
	}
}
