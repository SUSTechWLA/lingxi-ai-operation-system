package tool

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

const (
	maxCanonicalSchemaBytes = 256 << 10
	maxCanonicalSchemaDepth = 64
)

var (
	ErrInputSchemaInvalid  = errors.New("input schema invalid")
	ErrOutputSchemaInvalid = errors.New("output schema invalid")
	canonicalSchemaCache   sync.Map
)

type ContractValidationError struct {
	Kind  error
	Field string
	Err   error
}

func (e *ContractValidationError) Error() string {
	if e == nil {
		return "tool contract validation failed"
	}
	prefix := "tool contract validation failed"
	if e.Kind != nil {
		prefix = e.Kind.Error()
	}
	if e.Field != "" {
		prefix += " (" + e.Field + ")"
	}
	if e.Err == nil {
		return prefix
	}
	return prefix + ": " + e.Err.Error()
}

func (e *ContractValidationError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return []error{e.Kind, e.Err}
}

type CanonicalSchemaValidator struct {
	resolved *jsonschema.Resolved
}

func (v *CanonicalSchemaValidator) Validate(instance interface{}) error {
	if v == nil || v.resolved == nil {
		return errors.New("canonical schema validator is not initialized")
	}
	return v.resolved.Validate(instance)
}

func CompileCanonicalSchema(schema map[string]interface{}) (*CanonicalSchemaValidator, error) {
	if schema == nil {
		return nil, errors.New("canonical schema is nil")
	}
	if draft, ok := schema["$schema"].(string); ok && !supportedCanonicalSchemaDraft(draft) {
		return nil, fmt.Errorf("unsupported JSON Schema draft %q; only Draft 7 and Draft 2020-12 are supported", draft)
	}
	if err := checkSchemaShape(reflect.ValueOf(schema), 0, map[visitKey]bool{}); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("encode canonical schema: %w", err)
	}
	if len(encoded) > maxCanonicalSchemaBytes {
		return nil, fmt.Errorf("canonical schema exceeds %d bytes", maxCanonicalSchemaBytes)
	}
	hash := sha256.Sum256(encoded)
	if cached, ok := canonicalSchemaCache.Load(hash); ok {
		return cached.(*CanonicalSchemaValidator), nil
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, fmt.Errorf("decode canonical schema: %w", err)
	}
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical schema (remote references are disabled): %w", err)
	}
	validator := &CanonicalSchemaValidator{resolved: resolved}
	actual, _ := canonicalSchemaCache.LoadOrStore(hash, validator)
	return actual.(*CanonicalSchemaValidator), nil
}

func supportedCanonicalSchemaDraft(draft string) bool {
	draft = strings.TrimSuffix(strings.TrimSpace(draft), "#")
	switch draft {
	case "http://json-schema.org/draft-07/schema",
		"https://json-schema.org/draft-07/schema",
		"https://json-schema.org/draft/2020-12/schema":
		return true
	default:
		return false
	}
}

type visitKey struct {
	kind reflect.Kind
	ptr  uintptr
}

func checkSchemaShape(value reflect.Value, depth int, visiting map[visitKey]bool) error {
	if !value.IsValid() {
		return nil
	}
	if depth > maxCanonicalSchemaDepth {
		return fmt.Errorf("canonical schema exceeds maximum depth %d", maxCanonicalSchemaDepth)
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		key := visitKey{kind: value.Kind(), ptr: value.Pointer()}
		if visiting[key] {
			return errors.New("canonical schema contains a reference cycle")
		}
		visiting[key] = true
		defer delete(visiting, key)
		iter := value.MapRange()
		for iter.Next() {
			if err := checkSchemaShape(iter.Value(), depth+1, visiting); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		key := visitKey{kind: value.Kind(), ptr: value.Pointer()}
		if key.ptr != 0 {
			if visiting[key] {
				return errors.New("canonical schema contains a reference cycle")
			}
			visiting[key] = true
			defer delete(visiting, key)
		}
		for i := 0; i < value.Len(); i++ {
			if err := checkSchemaShape(value.Index(i), depth+1, visiting); err != nil {
				return err
			}
		}
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := checkSchemaShape(value.Index(i), depth+1, visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

func ValidateManifestContracts(manifest *ToolManifest) error {
	if manifest == nil {
		return &ContractValidationError{Kind: ErrInputSchemaInvalid, Field: "manifest", Err: errors.New("manifest is nil")}
	}
	if manifest.InputSchema != nil {
		if _, err := CompileCanonicalSchema(manifest.InputSchema); err != nil {
			return &ContractValidationError{Kind: ErrInputSchemaInvalid, Field: "input_schema", Err: err}
		}
	}
	if manifest.OutputSchema != nil {
		if _, err := CompileCanonicalSchema(manifest.OutputSchema); err != nil {
			return &ContractValidationError{Kind: ErrOutputSchemaInvalid, Field: "output_schema", Err: err}
		}
	}
	return nil
}

func ValidateManifestInput(manifest *ToolManifest, input interface{}) error {
	if manifest == nil || manifest.InputSchema == nil {
		return nil
	}
	validator, err := CompileCanonicalSchema(manifest.InputSchema)
	if err == nil {
		err = validator.Validate(input)
	}
	if err != nil {
		return &ContractValidationError{Kind: ErrInputSchemaInvalid, Field: "input", Err: err}
	}
	return nil
}

func ValidateManifestOutput(manifest *ToolManifest, output interface{}) error {
	if manifest == nil || manifest.OutputSchema == nil {
		return nil
	}
	validator, err := CompileCanonicalSchema(manifest.OutputSchema)
	if err == nil {
		err = validator.Validate(output)
	}
	if err != nil {
		return &ContractValidationError{Kind: ErrOutputSchemaInvalid, Field: "output", Err: err}
	}
	return nil
}

func ValidateLocalJobOutput(manifest *ToolManifest, output map[string]interface{}) error {
	if manifest == nil || manifest.OutputSchema == nil {
		return nil
	}
	value := interface{}(output)
	if isMCPContractManifest(manifest) {
		structured, ok := output["structuredContent"]
		if !ok || structured == nil {
			return &ContractValidationError{Kind: ErrOutputSchemaInvalid, Field: "structuredContent", Err: errors.New("MCP result is missing structuredContent")}
		}
		value = structured
	}
	return ValidateManifestOutput(manifest, value)
}

func isMCPContractManifest(manifest *ToolManifest) bool {
	if manifest == nil {
		return false
	}
	if manifest.Boundary == BoundaryMCPProvider || manifest.ProviderBinding != nil {
		return true
	}
	if manifest.Transport != nil {
		transportType := strings.ToLower(strings.TrimSpace(manifest.Transport.Type))
		return strings.Contains(transportType, "mcp")
	}
	return false
}
