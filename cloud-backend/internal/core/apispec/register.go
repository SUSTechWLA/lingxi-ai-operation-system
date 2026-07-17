package apispec

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Register mounts the OpenAPI spec and Swagger UI endpoints onto a Gin engine.
//
// Endpoints registered:
//   - GET /openapi.json  — OpenAPI 3.0 spec as JSON
//   - GET /openapi.yaml  — OpenAPI 3.0 spec as YAML (stub — returns JSON for now)
//   - GET /docs          — Swagger UI
func Register(r *gin.Engine, spec *Spec) {
	specJSON, _ := json.MarshalIndent(spec, "", "  ")

	r.GET("/openapi.json", func(c *gin.Context) {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Data(http.StatusOK, "application/json", specJSON)
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		// For now serve JSON; a YAML serializer can be added later.
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Data(http.StatusOK, "application/json", specJSON)
	})

	r.GET("/docs", gin.WrapH(SwaggerUIHandler("/openapi.json")))
}
