package skillruntime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCatalogReturnsSkillSummariesWithoutStages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := NewRegistry()
	if err := reg.Register(&SkillManifest{
		Name:        "create-opinion-videos",
		Version:     "1.0.0",
		Description: "口播观点输出视频流水线",
		Category:    "video",
		Health:      HealthHealthy,
		Stages: []StageDefinition{
			{Name: "viewpoint_dossier", Instruction: "viewpoint.md"},
			{Name: "recording_script", Instruction: "script.md", ApprovalReq: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	NewHandler(reg).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/skills/catalog", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Code int `json:"code"`
		Data struct {
			Skills []map[string]interface{} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(body.Data.Skills))
	}
	summary := body.Data.Skills[0]
	if summary["name"] != "create-opinion-videos" {
		t.Fatalf("unexpected summary name: %v", summary["name"])
	}
	if summary["stageCount"] != float64(2) {
		t.Fatalf("expected stageCount 2, got %v", summary["stageCount"])
	}
	if summary["requiresApproval"] != true {
		t.Fatalf("expected requiresApproval true, got %v", summary["requiresApproval"])
	}
	if _, ok := summary["stages"]; ok {
		t.Fatalf("catalog response must not include full stages: %v", summary)
	}
}

func TestRouteSelectsCanonicalTalkingHeadSkill(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := NewRegistry()
	if err := reg.Register(&SkillManifest{
		Name:        "create-opinion-videos",
		Version:     "1.0.0",
		DisplayName: "口播 / 知识视频",
		Description: "口播 / 知识视频",
		Category:    "video",
		Health:      HealthHealthy,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&SkillManifest{
		Name:           "voice-visual-video",
		Version:        "1.0.0",
		Category:       "video",
		Health:         HealthHealthy,
		Visibility:     VisibilityHidden,
		CanonicalSkill: "create-opinion-videos@1.0.0",
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	NewHandler(reg).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/skills/route", strings.NewReader(`{"brief":"把我的观点做成60秒口播视频，输出标题简介关键词"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Skill    map[string]interface{} `json:"skill"`
			Route    string                 `json:"route"`
			Source   string                 `json:"source"`
			Duration int                    `json:"targetDurationSec"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Skill["name"] != "create-opinion-videos" {
		t.Fatalf("expected canonical talking-head skill, got %v", body.Data.Skill["name"])
	}
	if body.Data.Route != "talking_head" {
		t.Fatalf("expected talking_head route, got %s", body.Data.Route)
	}
	if body.Data.Source != "fallback" {
		t.Fatalf("expected fallback source without LLM config, got %s", body.Data.Source)
	}
	if body.Data.Duration != 60 {
		t.Fatalf("expected duration parsed from brief, got %d", body.Data.Duration)
	}
}

func TestCatalogHidesHiddenSkillsByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := NewRegistry()
	if err := reg.Register(&SkillManifest{
		Name:        "create-opinion-videos",
		Version:     "1.0.0",
		Description: "口播 / 知识视频",
		Category:    "video",
		Health:      HealthHealthy,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&SkillManifest{
		Name:           "voice-visual-video",
		Version:        "1.0.0",
		Description:    "旧版口播可视化视频",
		Category:       "video",
		Health:         HealthHealthy,
		Visibility:     VisibilityHidden,
		CanonicalSkill: "create-opinion-videos@1.0.0",
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	NewHandler(reg).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/skills/catalog", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Skills []map[string]interface{} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Skills) != 1 {
		t.Fatalf("expected only canonical skill, got %d: %v", len(body.Data.Skills), body.Data.Skills)
	}
	if body.Data.Skills[0]["name"] != "create-opinion-videos" {
		t.Fatalf("expected canonical skill in catalog, got %v", body.Data.Skills[0]["name"])
	}
}

func TestCatalogCanIncludeHiddenSkillsForDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := NewRegistry()
	if err := reg.Register(&SkillManifest{Name: "visible", Version: "1.0.0", Health: HealthHealthy}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&SkillManifest{
		Name:           "hidden",
		Version:        "1.0.0",
		Health:         HealthHealthy,
		Visibility:     VisibilityHidden,
		CanonicalSkill: "visible@1.0.0",
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	NewHandler(reg).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/skills/catalog?includeHidden=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Skills []map[string]interface{} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Skills) != 2 {
		t.Fatalf("expected visible and hidden skills, got %d: %v", len(body.Data.Skills), body.Data.Skills)
	}
}
