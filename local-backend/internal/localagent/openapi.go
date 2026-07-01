// Package localagent — OpenAPI spec builder (inline, zero new deps).
package localagent

// LocalSpec is the OpenAPI 3.0 document for the local agent.
type LocalSpec struct {
	OpenAPI string                        `json:"openapi"`
	Info    LocalInfo                     `json:"info"`
	Servers []LocalServer                 `json:"servers,omitempty"`
	Paths   map[string]map[string]LocalOp `json:"paths"`
}

type LocalInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type LocalServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type LocalOp struct {
	OperationID string                    `json:"operationId"`
	Summary     string                    `json:"summary"`
	Description string                    `json:"description,omitempty"`
	Tags        []string                  `json:"tags,omitempty"`
	Parameters  []LocalParam              `json:"parameters,omitempty"`
	RequestBody *LocalRequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*LocalResponse `json:"responses"`
}

type LocalParam struct {
	Name        string       `json:"name"`
	In          string       `json:"in"`
	Description string       `json:"description,omitempty"`
	Required    bool         `json:"required"`
	Schema      *LocalSchema `json:"schema,omitempty"`
}

type LocalRequestBody struct {
	Description string                     `json:"description,omitempty"`
	Required    bool                       `json:"required"`
	Content     map[string]*LocalMediaType `json:"content"`
}

type LocalResponse struct {
	Description string                     `json:"description"`
	Content     map[string]*LocalMediaType `json:"content,omitempty"`
}

type LocalMediaType struct {
	Schema *LocalSchema `json:"schema,omitempty"`
}

type LocalSchema struct {
	Type       string                  `json:"type,omitempty"`
	Format     string                  `json:"format,omitempty"`
	Properties map[string]*LocalSchema `json:"properties,omitempty"`
	Required   []string                `json:"required,omitempty"`
	Nullable   bool                    `json:"nullable,omitempty"`
	Items      *LocalSchema            `json:"items,omitempty"`
}

// BuildLocalSpec constructs the OpenAPI spec for the local agent.
func BuildLocalSpec() *LocalSpec {
	s := &LocalSpec{
		OpenAPI: "3.0.3",
		Info:    LocalInfo{Title: "Tangying Local Agent API", Version: "0.1.0"},
		Servers: []LocalServer{{URL: "http://localhost:9090", Description: "Local desktop agent"}},
		Paths:   map[string]map[string]LocalOp{},
	}

	s.add("GET", "/api/local/health", LocalOp{
		OperationID: "getLocalHealth",
		Summary:     "Agent health status",
		Tags:        []string{"Health"},
		Responses: map[string]*LocalResponse{
			"200": {Description: "OK — agent is running",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"status":       {Type: "string"},
						"service":      {Type: "string"},
						"cloudApiBase": {Type: "string"},
						"dataDir":      {Type: "string"},
						"os":           {Type: "string"},
						"arch":         {Type: "string"},
					},
				}}},
			},
		},
	})

	s.add("GET", "/api/local/paths", LocalOp{
		OperationID: "getLocalPaths",
		Summary:     "Get data directory paths",
		Tags:        []string{"Paths"},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Paths returned",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"dataDir":        {Type: "string"},
						"cacheDir":       {Type: "string"},
						"configDir":      {Type: "string"},
						"projectDir":     {Type: "string"},
						"artifactDir":    {Type: "string"},
						"logDir":         {Type: "string"},
						"diagnosticsDir": {Type: "string"},
					},
				}}},
			},
		},
	})

	s.add("POST", "/api/local/logs", LocalOp{
		OperationID: "postLocalLogs",
		Summary:     "Write a log entry",
		Tags:        []string{"Logs"},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"source":  {Type: "string"},
					"level":   {Type: "string"},
					"message": {Type: "string"},
				},
				Required: []string{"message"},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Logged"},
			"400": {Description: "Invalid payload"},
		},
	})

	s.add("GET", "/api/local/model-providers", LocalOp{
		OperationID: "getLocalModelProviders",
		Summary:     "Get model provider settings",
		Tags:        []string{"Model Providers"},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Provider settings",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"providers": {Type: "object"},
					},
				}}},
			},
		},
	})
	s.add("PUT", "/api/local/model-providers", LocalOp{
		OperationID: "putLocalModelProviders",
		Summary:     "Update model provider settings",
		Tags:        []string{"Model Providers"},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"providers": {Type: "object"},
				},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Updated"},
			"400": {Description: "Invalid payload"},
		},
	})

	biaoshuProjectSchema := &LocalSchema{
		Type: "object",
		Properties: map[string]*LocalSchema{
			"runId":              {Type: "string"},
			"projectName":        {Type: "string"},
			"bidFilePath":        {Type: "string"},
			"status":             {Type: "string"},
			"createdAt":          {Type: "string", Format: "date-time"},
			"updatedAt":          {Type: "string", Format: "date-time"},
			"artifactCount":      {Type: "integer"},
			"validArtifactCount": {Type: "integer"},
		},
	}
	s.add("GET", "/api/local/biaoshu-projects", LocalOp{
		OperationID: "getLocalBiaoshuProjects",
		Summary:     "List local bid-writing projects",
		Tags:        []string{"Biaoshu Projects"},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Project list",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"projects": {Type: "array", Items: biaoshuProjectSchema},
					},
				}}},
			},
		},
	})
	s.add("PUT", "/api/local/biaoshu-projects/:runId", LocalOp{
		OperationID: "putLocalBiaoshuProject",
		Summary:     "Create or update a local bid-writing project",
		Tags:        []string{"Biaoshu Projects"},
		Parameters: []LocalParam{
			{Name: "runId", In: "path", Required: true, Schema: &LocalSchema{Type: "string"}},
		},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content:  map[string]*LocalMediaType{"application/json": {Schema: biaoshuProjectSchema}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Project stored"},
			"400": {Description: "Invalid payload"},
		},
	})

	s.add("POST", "/api/local/biaoshu-artifacts/read", LocalOp{
		OperationID: "readLocalBiaoshuArtifact",
		Summary:     "Read a local bid-writing artifact file",
		Description: "Reads a text artifact from trusted local artifact roots for desktop preview.",
		Tags:        []string{"Biaoshu Projects"},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"filePath": {Type: "string"},
				},
				Required: []string{"filePath"},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Artifact content",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"filePath": {Type: "string"},
						"format":   {Type: "string"},
						"content":  {Type: "string"},
						"size":     {Type: "integer"},
					},
				}}},
			},
			"400": {Description: "Invalid or untrusted file path"},
		},
	})

	s.add("POST", "/api/local/biaoshu-artifacts/write", LocalOp{
		OperationID: "writeLocalBiaoshuArtifact",
		Summary:     "Write revised content to a local bid-writing artifact",
		Description: "Writes text content back to a trusted local artifact file. Supports .md, .txt, .json only. Returns 409 if expectedPreviousContent does not match.",
		Tags:        []string{"Biaoshu Projects"},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"filePath":                {Type: "string"},
					"content":                 {Type: "string"},
					"expectedPreviousContent": {Type: "string"},
				},
				Required: []string{"filePath", "content"},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "File written successfully"},
			"400": {Description: "Invalid payload or unsupported format"},
			"403": {Description: "File path outside trusted roots"},
			"409": {Description: "File has been modified since last read"},
		},
	})

	s.add("GET", "/api/local/biaoshu-conversations", LocalOp{
		OperationID: "getLocalBiaoshuConversation",
		Summary:     "Get conversation messages for a bid-writing artifact",
		Description: "Returns the thread ID and all messages stored for a given runId + artifactPath.",
		Tags:        []string{"Biaoshu Projects"},
		Parameters: []LocalParam{
			{Name: "runId", In: "query", Required: true, Schema: &LocalSchema{Type: "string"}},
			{Name: "artifactPath", In: "query", Required: true, Schema: &LocalSchema{Type: "string"}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Conversation messages",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"threadId": {Type: "string"},
						"messages": {Type: "array", Items: &LocalSchema{
							Type: "object",
							Properties: map[string]*LocalSchema{
								"id":        {Type: "string"},
								"role":      {Type: "string"},
								"content":   {Type: "string"},
								"createdAt": {Type: "string", Format: "date-time"},
							},
						}},
					},
				}}},
			},
			"400": {Description: "Missing or invalid parameters"},
		},
	})

	s.add("POST", "/api/local/biaoshu-conversations/messages", LocalOp{
		OperationID: "postLocalBiaoshuConversationMessage",
		Summary:     "Append a message to a bid-writing conversation",
		Description: "Appends a user/assistant/system message to the conversation store for a given artifact.",
		Tags:        []string{"Biaoshu Projects"},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"runId":        {Type: "string"},
					"artifactPath": {Type: "string"},
					"artifactKind": {Type: "string"},
					"role":         {Type: "string"},
					"content":      {Type: "string"},
				},
				Required: []string{"runId", "artifactPath", "role", "content"},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Message appended",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"threadId": {Type: "string"},
						"message":  {Type: "object"},
					},
				}}},
			},
			"400": {Description: "Invalid or missing parameters"},
		},
	})

	s.add("POST", "/api/local/artifacts", LocalOp{
		OperationID: "postLocalArtifact",
		Summary:     "Store a local artifact",
		Tags:        []string{"Artifacts"},
		RequestBody: &LocalRequestBody{
			Required: true,
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"id":            {Type: "string"},
					"projectId":     {Type: "string"},
					"storageRef":    {Type: "string"},
					"mimeType":      {Type: "string"},
					"content":       {Type: "string"},
					"contentBase64": {Type: "string"},
				},
				Required: []string{"id", "projectId"},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Stored"},
			"400": {Description: "Invalid payload"},
		},
	})
	s.add("GET", "/api/local/artifacts/:id", LocalOp{
		OperationID: "getLocalArtifact",
		Summary:     "Get a local artifact by ID",
		Tags:        []string{"Artifacts"},
		Parameters: []LocalParam{
			{Name: "id", In: "path", Required: true, Schema: &LocalSchema{Type: "string"}},
			{Name: "projectId", In: "query", Required: true, Schema: &LocalSchema{Type: "string"}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Artifact data"},
			"404": {Description: "Not found"},
		},
	})
	s.add("DELETE", "/api/local/artifacts/:id", LocalOp{
		OperationID: "deleteLocalArtifact",
		Summary:     "Delete a local artifact",
		Tags:        []string{"Artifacts"},
		Parameters: []LocalParam{
			{Name: "id", In: "path", Required: true, Schema: &LocalSchema{Type: "string"}},
			{Name: "projectId", In: "query", Required: true, Schema: &LocalSchema{Type: "string"}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Deleted"},
		},
	})

	s.add("DELETE", "/api/local/projects/:id", LocalOp{
		OperationID: "deleteLocalProject",
		Summary:     "Delete a local project and its artifacts",
		Tags:        []string{"Projects"},
		Parameters: []LocalParam{
			{Name: "id", In: "path", Required: true, Schema: &LocalSchema{Type: "string"}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Deleted"},
		},
	})

	s.add("POST", "/api/local/diagnostics", LocalOp{
		OperationID: "postLocalDiagnostics",
		Summary:     "Create a diagnostics ZIP",
		Tags:        []string{"Diagnostics"},
		RequestBody: &LocalRequestBody{
			Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
				Type: "object",
				Properties: map[string]*LocalSchema{
					"reason": {Type: "string"},
				},
			}}},
		},
		Responses: map[string]*LocalResponse{
			"200": {Description: "Diagnostics ZIP created",
				Content: map[string]*LocalMediaType{"application/json": {Schema: &LocalSchema{
					Type: "object",
					Properties: map[string]*LocalSchema{
						"path":      {Type: "string"},
						"createdAt": {Type: "string", Format: "date-time"},
					},
				}}},
			},
		},
	})

	return s
}

func (s *LocalSpec) add(method, path string, op LocalOp) {
	if s.Paths[path] == nil {
		s.Paths[path] = map[string]LocalOp{}
	}
	op.OperationID = s.uniqueID(method, path, op.OperationID)
	s.Paths[path][methodToKey(method)] = op
}

func methodToKey(method string) string {
	switch method {
	case "GET":
		return "get"
	case "POST":
		return "post"
	case "PUT":
		return "put"
	case "DELETE":
		return "delete"
	case "PATCH":
		return "patch"
	default:
		return method
	}
}

func (s *LocalSpec) uniqueID(method, path, defaultID string) string {
	if defaultID != "" {
		return defaultID
	}
	id := methodToKey(method)
	for _, ch := range path {
		switch ch {
		case '/':
			id += "_"
		case ':', '-':
			// skip
		default:
			id += string(ch)
		}
	}
	return id
}
