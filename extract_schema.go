package sense

import (
	"reflect"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
)

var schemaCache sync.Map // reflect.Type → anthropic.ToolInputSchemaParam

// schemaFor generates a JSON schema from a struct type T and caches it.
func schemaFor[T any]() anthropic.ToolInputSchemaParam {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if cached, ok := schemaCache.Load(t); ok {
		schema, _ := cached.(anthropic.ToolInputSchemaParam)
		return schema
	}

	schema := buildSchema(t)
	schemaCache.Store(t, schema)
	return schema
}

// buildSchema converts a reflect.Type (must be struct) into a ToolInputSchemaParam.
func buildSchema(t reflect.Type) anthropic.ToolInputSchemaParam {
	props := make(map[string]any)
	var required []string

	for i := range t.NumField() {
		field := t.Field(i) //nolint:gocritic // field used by pointer below after copy
		if !field.IsExported() {
			continue
		}

		name := fieldName(&field)
		if name == "-" {
			continue
		}

		ft := field.Type
		isPtr := ft.Kind() == reflect.Ptr
		if isPtr {
			ft = ft.Elem()
		}

		prop := typeSchema(ft)

		if desc := field.Tag.Get("sense"); desc != "" {
			prop["description"] = desc
		}

		props[name] = prop

		if !isPtr {
			required = append(required, name)
		}
	}

	return anthropic.ToolInputSchemaParam{
		Properties: props,
		Required:   required,
	}
}

// fieldName returns the JSON field name for a struct field.
func fieldName(f *reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return f.Name
	}
	return name
}

// schemaForValue generates a JSON schema from a concrete value (must be a pointer
// to a struct) and caches it. This is the runtime counterpart to the generic
// schemaFor[T](), used by the s.Extract(text, &dest) method.
func schemaForValue(dest any) anthropic.ToolInputSchemaParam {
	t := reflect.TypeOf(dest)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if cached, ok := schemaCache.Load(t); ok {
		schema, _ := cached.(anthropic.ToolInputSchemaParam)
		return schema
	}

	schema := buildSchema(t)
	schemaCache.Store(t, schema)
	return schema
}

// typeSchema returns the JSON schema representation for a Go type.
func typeSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		items := typeSchema(t.Elem())
		return map[string]any{"type": "array", "items": items}
	case reflect.Struct:
		schema := buildSchema(t)
		result := map[string]any{
			"type":       "object",
			"properties": schema.Properties,
		}
		if len(schema.Required) > 0 {
			result["required"] = schema.Required
		}
		return result
	case reflect.Ptr:
		return typeSchema(t.Elem())
	default:
		return map[string]any{"type": "string"}
	}
}
