package agentruntime

import (
	"fmt"
	"reflect"
	"sort"
)

type argumentReference struct {
	StepID     string
	Field      string
	Expression string
}

// argumentReferences recursively finds output references in JSON-like argument
// objects. Reflection is intentional here: planner code commonly uses both
// []interface{} and typed containers such as []map[string]interface{}.
func argumentReferences(value interface{}) []argumentReference {
	references := make([]argumentReference, 0)
	walkArgumentValues(reflect.ValueOf(value), func(expression string) {
		stepID, field, ok := outputReference(expression)
		if !ok {
			return
		}
		references = append(references, argumentReference{
			StepID:     stepID,
			Field:      field,
			Expression: expression,
		})
	})
	return references
}

func walkArgumentValues(value reflect.Value, visitString func(string)) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			walkArgumentValues(value.Elem(), visitString)
		}
	case reflect.String:
		visitString(value.String())
	case reflect.Map:
		keys := value.MapKeys()
		sort.SliceStable(keys, func(i, j int) bool {
			return argumentMapKey(keys[i]) < argumentMapKey(keys[j])
		})
		for _, key := range keys {
			walkArgumentValues(value.MapIndex(key), visitString)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			walkArgumentValues(value.Index(i), visitString)
		}
	}
}

func argumentMapKey(value reflect.Value) string {
	if value.Kind() == reflect.String {
		return value.String()
	}
	return fmt.Sprint(value.Interface())
}

// pruneRemovedStepReferences removes only leaves that reference compiler-
// removed steps. Unknown references not named in removedStepIDs are preserved
// so PlanGuard can diagnose them.
func pruneRemovedStepReferences(value interface{}, removedStepIDs map[string]bool) (interface{}, bool) {
	pruned, keep := pruneRemovedStepReferenceValue(reflect.ValueOf(value), removedStepIDs)
	if !keep {
		return nil, false
	}
	if !pruned.IsValid() {
		return nil, true
	}
	return pruned.Interface(), true
}

func pruneRemovedStepReferenceValue(value reflect.Value, removedStepIDs map[string]bool) (reflect.Value, bool) {
	if !value.IsValid() {
		return value, true
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value, true
		}
		child, keep := pruneRemovedStepReferenceValue(value.Elem(), removedStepIDs)
		if !keep {
			return reflect.Value{}, false
		}
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(child)
		return wrapped, true
	case reflect.Pointer:
		if value.IsNil() {
			return value, true
		}
		child, keep := pruneRemovedStepReferenceValue(value.Elem(), removedStepIDs)
		if !keep {
			return reflect.Value{}, false
		}
		wrapped := reflect.New(value.Type().Elem())
		wrapped.Elem().Set(child)
		return wrapped, true
	case reflect.String:
		stepID, _, ok := outputReference(value.String())
		if ok && removedStepIDs[stepID] {
			return reflect.Value{}, false
		}
		return value, true
	case reflect.Map:
		if value.IsNil() {
			return value, true
		}
		pruned := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			child, keep := pruneRemovedStepReferenceValue(value.MapIndex(key), removedStepIDs)
			if keep {
				pruned.SetMapIndex(key, child)
			}
		}
		return pruned, true
	case reflect.Slice:
		if value.IsNil() {
			return value, true
		}
		pruned := reflect.MakeSlice(value.Type(), 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			child, keep := pruneRemovedStepReferenceValue(value.Index(i), removedStepIDs)
			if keep {
				pruned = reflect.Append(pruned, child)
			}
		}
		return pruned, true
	case reflect.Array:
		pruned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			child, keep := pruneRemovedStepReferenceValue(value.Index(i), removedStepIDs)
			if !keep {
				return value, true
			}
			pruned.Index(i).Set(child)
		}
		return pruned, true
	default:
		return value, true
	}
}
