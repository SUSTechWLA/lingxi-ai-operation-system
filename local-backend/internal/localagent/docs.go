package localagent

import (
	"encoding/json"
	"net/http"
)

// SpecJSON returns the OpenAPI 3.0 spec as JSON bytes.
func SpecJSON() []byte {
	data, _ := json.MarshalIndent(BuildLocalSpec(), "", "  ")
	return data
}

// SpecSpec() serves the OpenAPI JSON.
func handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(SpecJSON())
}

// handleDocs serves the Swagger UI page pointing at the OpenAPI spec.
func handleDocs(w http.ResponseWriter, r *http.Request) {
	specURL := "/api/local/openapi.json"
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Local Agent API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: "` + specURL + `",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
        layout: "StandaloneLayout"
      });
    };
  </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}
