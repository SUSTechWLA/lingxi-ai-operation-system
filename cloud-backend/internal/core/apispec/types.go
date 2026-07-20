// Package apispec provides a zero-dependency OpenAPI 3.0 specification builder
// for the cloud-backend. The spec is constructed programmatically (not from
// annotations) and served as JSON / YAML alongside a Swagger UI.
package apispec

// Spec is the root OpenAPI 3.0 document.
type Spec struct {
	OpenAPI    string               `json:"openapi"`
	Info       Info                 `json:"info"`
	Servers    []Server             `json:"servers,omitempty"`
	Tags       []Tag                `json:"tags,omitempty"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components,omitempty"`
}

// Info describes the API.
type Info struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version"`
	Contact     *Contact `json:"contact,omitempty"`
	License     *License `json:"license,omitempty"`
}

type Contact struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

type License struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PathItem groups operations for a single path.
type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Options *Operation `json:"options,omitempty"`
}

// Operation describes a single API operation.
type Operation struct {
	OperationID string               `json:"operationId"`
	Summary     string               `json:"summary"`
	Description string               `json:"description,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	Parameters  []Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses"`
	Deprecated  bool                 `json:"deprecated,omitempty"`
}

type Parameter struct {
	Name        string     `json:"name"`
	In          string     `json:"in"` // query, path, header, cookie
	Description string     `json:"description,omitempty"`
	Required    bool       `json:"required"`
	Schema      *SchemaRef `json:"schema,omitempty"`
	Style       string     `json:"style,omitempty"`
	Explode     *bool      `json:"explode,omitempty"`
}

type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required"`
	Content     map[string]*MediaType `json:"content"`
}

type Response struct {
	Description string                `json:"description"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *SchemaRef `json:"schema,omitempty"`
}

// SchemaRef is a $ref or inline schema.
type SchemaRef struct {
	Ref    string  `json:"$ref,omitempty"`
	Schema *Schema `json:"schema,omitempty"` // for inline schemas
}

// Schema represents a JSON Schema object used in components/schemas and
// operation request/response bodies.
type Schema struct {
	Type                 string                `json:"type,omitempty"`
	Format               string                `json:"format,omitempty"`
	Description          string                `json:"description,omitempty"`
	Items                *SchemaRef            `json:"items,omitempty"`
	Properties           map[string]*SchemaRef `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties *SchemaRef            `json:"additionalProperties,omitempty"`
	AllOf                []*SchemaRef          `json:"allOf,omitempty"`
	Ref                  string                `json:"$ref,omitempty"`
	Enum                 []any                 `json:"enum,omitempty"`
	Minimum              *float64              `json:"minimum,omitempty"`
	Default              any                   `json:"default,omitempty"`
	Nullable             bool                  `json:"nullable,omitempty"`
	Deprecated           bool                  `json:"deprecated,omitempty"`
	Example              any                   `json:"example,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
	In          string `json:"in,omitempty"`
}
