package observability

import (
	"reflect"
	"strings"
	"unicode"
)

// Redact returns an independent event containing only fields in the v1 wire
// contract. Arbitrary evidence attributes and unsafe developer detail are
// removed, never masked into reusable text.
func Redact(event Event) Event {
	redacted := event
	redacted.Evidence.InputRefs = cloneStrings(event.Evidence.InputRefs)
	redacted.Evidence.OutputRefs = cloneStrings(event.Evidence.OutputRefs)
	redacted.Evidence.SizeBytes = cloneInt64(event.Evidence.SizeBytes)
	redacted.Evidence.Attributes = nil
	redacted.Privacy.RedactedFields = cloneStrings(event.Privacy.RedactedFields)
	if redacted.Privacy.RedactedFields == nil {
		redacted.Privacy.RedactedFields = []string{}
	}

	if event.Error != nil {
		errorCopy := *event.Error
		errorCopy.EvidenceRefs = cloneStrings(event.Error.EvidenceRefs)
		redacted.Error = &errorCopy
		if !matchesDiagnosticKey(errorCopy.Code, errorCopy.DeveloperDetail) {
			redacted.Error.DeveloperDetail = ""
			addRedactedField(&redacted.Privacy, "error.developerDetail")
		}
	}

	if len(event.Evidence.Attributes) > 0 {
		addRedactedField(&redacted.Privacy, "evidence.attributes")
	}

	return redacted
}

func ContainsSecret(value any) bool {
	return containsSecret(reflect.ValueOf(value), make(map[visit]struct{}))
}

type visit struct {
	kind reflect.Kind
	ptr  uintptr
}

func containsSecret(value reflect.Value, seen map[visit]struct{}) bool {
	if !value.IsValid() {
		return false
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Type() == reflect.TypeOf(PrivacyClassification("")) &&
		PrivacyClassification(value.String()) == PrivacySecret {
		return true
	}
	if value.Type() == reflect.TypeOf(EventError{}) {
		eventError := value.Interface().(EventError)
		if !matchesDiagnosticKey(eventError.Code, eventError.DeveloperDetail) {
			return true
		}
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return false
		}
		current := visit{kind: value.Kind(), ptr: value.Pointer()}
		if _, ok := seen[current]; ok {
			return false
		}
		seen[current] = struct{}{}
		return containsSecret(value.Elem(), seen)
	case reflect.Map:
		if value.IsNil() {
			return false
		}
		current := visit{kind: value.Kind(), ptr: value.Pointer()}
		if _, ok := seen[current]; ok {
			return false
		}
		seen[current] = struct{}{}
		if decodedDeveloperDetailMismatch(value) {
			return true
		}
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key()
			item := iter.Value()
			if key.Kind() == reflect.String {
				name := key.String()
				if normalizedFieldName(name) == "classification" {
					classification, ok := stringValue(item)
					if ok && strings.EqualFold(classification, string(PrivacySecret)) {
						return true
					}
				}
				if sensitiveField(name) && hasContent(item) {
					return true
				}
				if normalizedFieldName(name) == "developerdetail" && unsafeDeveloperDetail(item) {
					return true
				}
			}
			if containsSecret(item, seen) {
				return true
			}
		}
	case reflect.Struct:
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if !field.CanInterface() {
				continue
			}
			name := valueType.Field(i).Name
			if normalizedFieldName(name) == "developerdetail" && unsafeDeveloperDetail(field) {
				return true
			}
			if sensitiveField(name) && hasContent(field) {
				return true
			}
			if containsSecret(field, seen) {
				return true
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return false
		}
		current := visit{kind: value.Kind(), ptr: value.Pointer()}
		if _, ok := seen[current]; ok {
			return false
		}
		seen[current] = struct{}{}
		fallthrough
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if containsSecret(value.Index(i), seen) {
				return true
			}
		}
	}
	return false
}

func decodedDeveloperDetailMismatch(value reflect.Value) bool {
	var code string
	var detail string
	hasDetail := false
	iter := value.MapRange()
	for iter.Next() {
		key := iter.Key()
		if key.Kind() != reflect.String {
			continue
		}
		item, ok := stringValue(iter.Value())
		if !ok {
			continue
		}
		switch normalizedFieldName(key.String()) {
		case "code":
			code = item
		case "developerdetail":
			detail = item
			hasDetail = true
		}
	}
	return hasDetail && !matchesDiagnosticKey(code, detail)
}

func stringValue(value reflect.Value) (string, bool) {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.String {
		return "", false
	}
	return value.String(), true
}

func sensitiveField(name string) bool {
	normalized := normalizedFieldName(name)
	for _, suffix := range []string{
		"authorization",
		"cookie",
		"token",
		"password",
		"apikey",
		"secret",
		"prompt",
		"userinput",
	} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

func normalizedFieldName(name string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func unsafeDeveloperDetail(value reflect.Value) bool {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.String || value.Len() == 0 {
		return false
	}
	return !developerDetailPattern.MatchString(value.String())
}

func hasContent(value reflect.Value) bool {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return false
	}
	return !value.IsZero()
}

func addRedactedField(privacy *Privacy, field string) {
	for _, existing := range privacy.RedactedFields {
		if existing == field {
			return
		}
	}
	privacy.RedactedFields = append(privacy.RedactedFields, field)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
