package skillruntime

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
	"github.com/tangying-ai/aios-core/internal/core/config"
)

// CompileFunc converts a SkillManifest into a DAG JSON.
// This is injected to avoid a circular dependency (skillruntime ← workflow).
type CompileFunc func(skill *SkillManifest) (json.RawMessage, error)

// Handler serves HTTP endpoints for skill queries and compilation.
type Handler struct {
	reg     *Registry
	compile CompileFunc
	router  *SkillRouter
}

func NewHandler(reg *Registry) *Handler {
	return &Handler{reg: reg, router: NewSkillRouter(reg, config.OpenAIConfig{})}
}

// SetCompiler injects the skill-to-DAG compiler function.
func (h *Handler) SetCompiler(fn CompileFunc) {
	h.compile = fn
}

// SetOpenAIConfig enables LLM-based skill routing. Without an API key the router
// uses deterministic fallback rules.
func (h *Handler) SetOpenAIConfig(cfg config.OpenAIConfig) {
	h.router = NewSkillRouter(h.reg, cfg)
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/skills")
	{
		api.GET("", h.List)
		api.GET("/catalog", h.Catalog)
		api.POST("/route", h.Route)
		api.GET("/:name/:version", h.Get)
		api.POST("/:name/:version/compile", h.Compile)
	}
}

func ok(c *gin.Context, data interface{}) {
	httpx.OK(c, data)
}

func fail(c *gin.Context, status int, msg string) {
	httpx.Fail(c, status, msg)
}

// GET /api/skills
func (h *Handler) List(c *gin.Context) {
	skills := h.reg.List()
	health := h.reg.Health()
	ok(c, gin.H{"skills": skills, "health": health})
}

// GET /api/skills/catalog
func (h *Handler) Catalog(c *gin.Context) {
	includeHidden := c.Query("includeHidden") == "true"
	ok(c, gin.H{"skills": h.reg.Catalog(includeHidden), "health": h.reg.Health()})
}

// POST /api/skills/route
func (h *Handler) Route(c *gin.Context) {
	var req RouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	result, err := h.router.Route(c.Request.Context(), req)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	ok(c, result)
}

// GET /api/skills/:name/:version
func (h *Handler) Get(c *gin.Context) {
	skill, err := h.reg.Get(c.Param("name"), c.Param("version"))
	if err != nil {
		fail(c, 404, "skill not found")
		return
	}
	ok(c, gin.H{"skill": skill})
}

// POST /api/skills/:name/:version/compile
// Body (optional): {"register": true} — also register as workflow template
func (h *Handler) Compile(c *gin.Context) {
	skill, err := h.reg.Get(c.Param("name"), c.Param("version"))
	if err != nil {
		fail(c, 404, "skill not found")
		return
	}

	if h.compile == nil {
		fail(c, 501, "compiler not configured")
		return
	}

	dag, err := h.compile(skill)
	if err != nil {
		fail(c, 500, "compilation failed: "+err.Error())
		return
	}

	ok(c, gin.H{
		"skill":   skill.Name + "@" + skill.Version,
		"dag":     json.RawMessage(dag),
		"message": "DAG compiled successfully. Use POST /api/workflows to register as a template.",
	})
}
