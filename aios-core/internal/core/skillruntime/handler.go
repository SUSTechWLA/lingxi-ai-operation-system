package skillruntime

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CompileFunc converts a SkillManifest into a DAG JSON.
// This is injected to avoid a circular dependency (skillruntime ← workflow).
type CompileFunc func(skill *SkillManifest) (json.RawMessage, error)

// Handler serves HTTP endpoints for skill queries and compilation.
type Handler struct {
	reg     *Registry
	compile CompileFunc
}

func NewHandler(reg *Registry) *Handler {
	return &Handler{reg: reg}
}

// SetCompiler injects the skill-to-DAG compiler function.
func (h *Handler) SetCompiler(fn CompileFunc) {
	h.compile = fn
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/skills")
	{
		api.GET("", h.List)
		api.GET("/:name/:version", h.Get)
		api.POST("/:name/:version/compile", h.Compile)
	}
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg, "data": nil})
}

// GET /api/skills
func (h *Handler) List(c *gin.Context) {
	skills := h.reg.List()
	health := h.reg.Health()
	ok(c, gin.H{"skills": skills, "health": health})
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
