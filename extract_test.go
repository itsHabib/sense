package sense

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

type mountError struct {
	Device   string `json:"device" sense:"The device path"`
	VolumeID string `json:"volume_id"`
	Message  string `json:"message"`
}

type nestedAddr struct {
	City  string `json:"city"`
	State string `json:"state"`
}

type personRecord struct {
	Name    string     `json:"name"`
	Age     int        `json:"age"`
	Active  bool       `json:"active"`
	Address nestedAddr `json:"address"`
}

type optionalFields struct {
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

// --- Extract behavior tests ---

func TestExtract_Basic(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"device": "/dev/sdf",
			"volume_id": "vol-123",
			"message": "already mounted"
		}`),
		usage: &Usage{InputTokens: 200, OutputTokens: 50},
	}
	s := testSession(mock)

	result, err := Extract[mountError](s, "device /dev/sdf already mounted with vol-123").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Device != "/dev/sdf" {
		t.Errorf("expected /dev/sdf, got %s", result.Data.Device)
	}
	if result.Data.VolumeID != "vol-123" {
		t.Errorf("expected vol-123, got %s", result.Data.VolumeID)
	}
	if result.Data.Message != "already mounted" {
		t.Errorf("expected 'already mounted', got %s", result.Data.Message)
	}
	if result.TokensUsed != 250 {
		t.Errorf("expected 250 tokens, got %d", result.TokensUsed)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestExtract_NestedStruct(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"name": "Alice",
			"age": 30,
			"active": true,
			"address": {"city": "Portland", "state": "OR"}
		}`),
		usage: &Usage{InputTokens: 150, OutputTokens: 60},
	}
	s := testSession(mock)

	result, err := Extract[personRecord](s, "Alice, 30, lives in Portland OR, active member").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Name != "Alice" {
		t.Errorf("expected Alice, got %s", result.Data.Name)
	}
	if result.Data.Age != 30 {
		t.Errorf("expected 30, got %d", result.Data.Age)
	}
	if !result.Data.Active {
		t.Error("expected active=true")
	}
	if result.Data.Address.City != "Portland" {
		t.Errorf("expected Portland, got %s", result.Data.Address.City)
	}
}

func TestExtract_OptionalFields(t *testing.T) {
	email := "alice@example.com"
	mock := &mockCaller{
		response: json.RawMessage(`{
			"name": "Alice",
			"email": "alice@example.com",
			"phone": null
		}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 40},
	}
	s := testSession(mock)

	result, err := Extract[optionalFields](s, "Alice, email alice@example.com").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Name != "Alice" {
		t.Errorf("expected Alice, got %s", result.Data.Name)
	}
	if result.Data.Email == nil || *result.Data.Email != email {
		t.Errorf("expected email=%s, got %v", email, result.Data.Email)
	}
	if result.Data.Phone != nil {
		t.Errorf("expected nil phone, got %v", result.Data.Phone)
	}
}

func TestExtract_EmptyText(t *testing.T) {
	s := testSession(&mockCaller{})
	_, err := Extract[mountError](s, "").Run()
	if !errors.Is(err, ErrNoText) {
		t.Errorf("expected ErrNoText, got %v", err)
	}
}

func TestExtract_ClientError(t *testing.T) {
	mock := &mockCaller{err: errors.New("connection refused")}
	s := testSession(mock)

	_, err := Extract[mountError](s, "some text").Run()
	if err == nil {
		t.Fatal("expected error")
	}
	var senseErr *Error
	if !errors.As(err, &senseErr) {
		t.Fatalf("expected *sense.Error, got %T", err)
	}
	if senseErr.Op != "extract" {
		t.Errorf("expected op=extract, got %s", senseErr.Op)
	}
}

func TestExtract_BadJSON(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{not valid}`),
		usage:    &Usage{InputTokens: 100, OutputTokens: 30},
	}
	s := testSession(mock)

	_, err := Extract[mountError](s, "some text").Run()
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}

	u := s.Usage()
	if u.Calls != 1 {
		t.Errorf("expected 1 call recorded, got %d", u.Calls)
	}
}

func TestExtract_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")
	s := testSession(&mockCaller{})

	result, err := Extract[mountError](s, "anything").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Device != "" {
		t.Errorf("expected zero value, got %s", result.Data.Device)
	}
}

func TestExtract_ContextInPrompt(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device":"x","volume_id":"y","message":"z"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	_, _ = Extract[mountError](s, "some error").
		Context("AWS EC2 EBS mount errors").
		Run()

	if mock.lastReq == nil {
		t.Fatal("expected a call")
	}
	msg := mock.lastReq.userMessage
	if !contains(msg, "AWS EC2 EBS mount errors") {
		t.Errorf("prompt missing context:\n%s", msg)
	}
	if !contains(msg, "some error") {
		t.Errorf("prompt missing text:\n%s", msg)
	}
	if mock.lastReq.toolName != "submit_extraction" {
		t.Errorf("expected tool name submit_extraction, got %s", mock.lastReq.toolName)
	}
}

func TestExtract_ModelOverride(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device":"x","volume_id":"y","message":"z"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	_, _ = Extract[mountError](s, "text").
		Model("claude-haiku-4-5-20251001").
		Run()

	if mock.lastReq.model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected model override, got %s", mock.lastReq.model)
	}
}

func TestExtract_RecordsUsage(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device":"x","volume_id":"y","message":"z"}`),
		usage:    &Usage{InputTokens: 300, OutputTokens: 100},
	}
	s := testSession(mock)

	_, _ = Extract[mountError](s, "text").Run()

	u := s.Usage()
	if u.Calls != 1 {
		t.Errorf("expected 1 call, got %d", u.Calls)
	}
	if u.InputTokens != 300 {
		t.Errorf("expected 300 input tokens, got %d", u.InputTokens)
	}
}

func TestExtract_ConcurrentUsage(t *testing.T) {
	mock := &concurrentMockCaller{
		response: json.RawMessage(`{"device":"x","volume_id":"y","message":"z"}`),
		usage:    &Usage{InputTokens: 10, OutputTokens: 5},
	}
	s := &Session{
		client:     mock,
		model:      "claude-sonnet-4-6",
		timeout:    0,
		maxRetries: 3,
	}

	const goroutines = 50
	done := make(chan struct{}, goroutines)
	for range goroutines {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = Extract[mountError](s, "text").Run()
		}()
	}
	for range goroutines {
		<-done
	}

	u := s.Usage()
	if u.Calls != goroutines {
		t.Errorf("expected %d calls, got %d", goroutines, u.Calls)
	}
}

// --- Schema: type mapping ---

func TestSchema_TypeMapping(t *testing.T) {
	t.Parallel()

	type allTypes struct {
		S   string  `json:"s"`
		B   bool    `json:"b"`
		I   int     `json:"i"`
		I8  int8    `json:"i8"`
		I16 int16   `json:"i16"`
		I32 int32   `json:"i32"`
		I64 int64   `json:"i64"`
		U   uint    `json:"u"`
		U8  uint8   `json:"u8"`
		U16 uint16  `json:"u16"`
		U32 uint32  `json:"u32"`
		U64 uint64  `json:"u64"`
		F32 float32 `json:"f32"`
		F64 float64 `json:"f64"`
	}

	schema := schemaFor[allTypes]()
	props := schemaProps(t, &schema)

	tests := []struct {
		field    string
		wantType string
	}{
		{"s", "string"},
		{"b", "boolean"},
		{"i", "integer"},
		{"i8", "integer"},
		{"i16", "integer"},
		{"i32", "integer"},
		{"i64", "integer"},
		{"u", "integer"},
		{"u8", "integer"},
		{"u16", "integer"},
		{"u32", "integer"},
		{"u64", "integer"},
		{"f32", "number"},
		{"f64", "number"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()
			assertPropType(t, props, tt.field, tt.wantType)
		})
	}

	assertRequired(t, &schema, "s", "b", "i", "i8", "i16", "i32", "i64",
		"u", "u8", "u16", "u32", "u64", "f32", "f64")
}

// --- Schema: JSON tag handling ---

func TestSchema_JSONTags(t *testing.T) {
	t.Parallel()

	t.Run("rename", func(t *testing.T) {
		t.Parallel()
		type s struct {
			FirstName string `json:"first_name"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if _, ok := props["first_name"]; !ok {
			t.Error("expected first_name from json tag")
		}
		if _, ok := props["FirstName"]; ok {
			t.Error("should use json tag name, not Go field name")
		}
	})

	t.Run("ignore_dash", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Visible string `json:"visible"`
			Hidden  string `json:"-"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if _, ok := props["Hidden"]; ok {
			t.Error("json:\"-\" field should be excluded")
		}
		if _, ok := props["-"]; ok {
			t.Error("json:\"-\" should not create a property named -")
		}
		if len(props) != 1 {
			t.Errorf("expected 1 property, got %d", len(props))
		}
	})

	t.Run("omitempty_keeps_field", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Name string `json:"name,omitempty"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if _, ok := props["name"]; !ok {
			t.Error("omitempty should not affect schema — field should be present")
		}
	})

	t.Run("no_tag_uses_go_name", func(t *testing.T) {
		t.Parallel()
		type s struct {
			GoFieldName string
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if _, ok := props["GoFieldName"]; !ok {
			t.Error("field without json tag should use Go field name")
		}
	})

	t.Run("empty_name_uses_go_name", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Name string `json:",omitempty"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if _, ok := props["Name"]; !ok {
			t.Error("empty json tag name should fall back to Go field name")
		}
	})
}

// --- Schema: sense description tag ---

func TestSchema_SenseDescriptions(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		schema := schemaFor[mountError]()
		props := schemaProps(t, &schema)

		prop, ok := props["device"].(map[string]any)
		if !ok {
			t.Fatal("expected device property")
		}
		desc, ok := prop["description"].(string)
		if !ok || desc != "The device path" {
			t.Errorf("expected description 'The device path', got %v", desc)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		schema := schemaFor[mountError]()
		props := schemaProps(t, &schema)

		prop, ok := props["volume_id"].(map[string]any)
		if !ok {
			t.Fatal("expected volume_id property")
		}
		if _, hasDesc := prop["description"]; hasDesc {
			t.Error("field without sense tag should not have description")
		}
	})

	t.Run("multiple", func(t *testing.T) {
		t.Parallel()
		type s struct {
			IP   string `json:"ip" sense:"Source IP address"`
			Port int    `json:"port" sense:"Port number (1-65535)"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)

		ipProp, ok := props["ip"].(map[string]any)
		if !ok {
			t.Fatal("expected ip property")
		}
		if ipProp["description"] != "Source IP address" {
			t.Errorf("wrong ip description: %v", ipProp["description"])
		}
		portProp, ok := props["port"].(map[string]any)
		if !ok {
			t.Fatal("expected port property")
		}
		if portProp["description"] != "Port number (1-65535)" {
			t.Errorf("wrong port description: %v", portProp["description"])
		}
	})
}

// --- Schema: pointer/optional fields ---

func TestSchema_PointerTypes(t *testing.T) {
	t.Parallel()

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Name  string  `json:"name"`
			Email *string `json:"email"`
		}
		schema := schemaFor[s]()
		assertRequired(t, &schema, "name")
		assertNotRequired(t, &schema, "email")
	})

	t.Run("int", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Age *int `json:"age"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertPropType(t, props, "age", "integer")
		assertNotRequired(t, &schema, "age")
	})

	t.Run("bool", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Active *bool `json:"active"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertPropType(t, props, "active", "boolean")
		assertNotRequired(t, &schema, "active")
	})

	t.Run("float64", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Score *float64 `json:"score"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertPropType(t, props, "score", "number")
		assertNotRequired(t, &schema, "score")
	})
}

func TestSchema_PointerRequired(t *testing.T) {
	t.Parallel()

	t.Run("all_pointers_none_required", func(t *testing.T) {
		t.Parallel()
		type s struct {
			A *string  `json:"a"`
			B *int     `json:"b"`
			C *bool    `json:"c"`
			D *float64 `json:"d"`
		}
		schema := schemaFor[s]()
		if len(schema.Required) != 0 {
			t.Errorf("expected no required fields, got %v", schema.Required)
		}
	})

	t.Run("mixed_required_optional", func(t *testing.T) {
		t.Parallel()
		type s struct {
			ID     int     `json:"id"`
			Name   string  `json:"name"`
			Email  *string `json:"email"`
			Phone  *string `json:"phone"`
			Active bool    `json:"active"`
			Score  *int    `json:"score"`
		}
		schema := schemaFor[s]()
		assertRequired(t, &schema, "id", "name", "active")
		assertNotRequired(t, &schema, "email", "phone", "score")
	})

	t.Run("pointer_to_struct", func(t *testing.T) {
		t.Parallel()
		type inner struct {
			Val string `json:"val"`
		}
		type outer struct {
			Nested *inner `json:"nested"`
		}
		schema := schemaFor[outer]()
		props := schemaProps(t, &schema)

		prop := mustNested(t, props, "nested")
		if prop["type"] != "object" {
			t.Errorf("expected object for pointer-to-struct, got %v", prop["type"])
		}
		nested := mustNested(t, props, "nested", "properties")
		if _, ok := nested["val"]; !ok {
			t.Error("expected val in pointer-to-struct properties")
		}
		assertNotRequired(t, &schema, "nested")
	})

	t.Run("pointer_to_slice", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Tags *[]string `json:"tags"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)

		prop := mustNested(t, props, "tags")
		if prop["type"] != "array" {
			t.Errorf("expected array for pointer-to-slice, got %v", prop["type"])
		}
		assertNotRequired(t, &schema, "tags")
	})
}

// --- Schema: slices ---

func TestSchema_SliceElementTypes(t *testing.T) {
	t.Parallel()

	t.Run("strings", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Tags []string `json:"tags"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertArrayItems(t, props, "tags", "string")
		assertRequired(t, &schema, "tags")
	})

	t.Run("ints", func(t *testing.T) {
		t.Parallel()
		type s struct {
			IDs []int `json:"ids"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertArrayItems(t, props, "ids", "integer")
	})

	t.Run("floats", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Scores []float64 `json:"scores"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertArrayItems(t, props, "scores", "number")
	})

	t.Run("bools", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Flags []bool `json:"flags"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		assertArrayItems(t, props, "flags", "boolean")
	})
}

func TestSchema_SliceComplex(t *testing.T) {
	t.Parallel()

	t.Run("structs", func(t *testing.T) {
		t.Parallel()
		type item struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}
		type s struct {
			Items []item `json:"items"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)

		itemsSchema := mustNested(t, props, "items", "items")
		if itemsSchema["type"] != "object" {
			t.Errorf("expected object items, got %v", itemsSchema["type"])
		}
		nested := mustNested(t, props, "items", "items", "properties")
		if _, ok := nested["name"]; !ok {
			t.Error("expected name in struct item properties")
		}
		if _, ok := nested["value"]; !ok {
			t.Error("expected value in struct item properties")
		}
	})

	t.Run("slice_of_slices", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Matrix [][]int `json:"matrix"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)

		outerItems := mustNested(t, props, "matrix")
		if outerItems["type"] != "array" {
			t.Fatal("expected outer array")
		}
		innerItems := mustNested(t, props, "matrix", "items")
		if innerItems["type"] != "array" {
			t.Fatal("expected inner array")
		}
		leaf := mustNested(t, props, "matrix", "items", "items")
		if leaf["type"] != "integer" {
			t.Errorf("expected integer leaf items, got %v", leaf["type"])
		}
	})
}

// --- Schema: nested structs ---

func TestSchema_NestedBasic(t *testing.T) {
	t.Parallel()

	t.Run("fields_and_type", func(t *testing.T) {
		t.Parallel()
		schema := schemaFor[personRecord]()
		props := schemaProps(t, &schema)

		assertPropType(t, props, "address", "object")
		nested := mustNested(t, props, "address", "properties")
		if _, ok := nested["city"]; !ok {
			t.Error("expected city in nested properties")
		}
		if _, ok := nested["state"]; !ok {
			t.Error("expected state in nested properties")
		}
	})

	t.Run("propagates_required", func(t *testing.T) {
		t.Parallel()
		schema := schemaFor[personRecord]()
		props := schemaProps(t, &schema)

		prop, ok := props["address"].(map[string]any)
		if !ok {
			t.Fatal("expected address property")
		}
		req, ok := prop["required"].([]string)
		if !ok {
			t.Fatal("expected required in nested struct")
		}
		reqSet := make(map[string]bool)
		for _, r := range req {
			reqSet[r] = true
		}
		if !reqSet["city"] || !reqSet["state"] {
			t.Errorf("expected city and state required, got %v", req)
		}
	})

	t.Run("three_levels_deep", func(t *testing.T) {
		t.Parallel()
		type inner struct {
			Value string `json:"value"`
		}
		type middle struct {
			Inner inner `json:"inner"`
		}
		type outer struct {
			Middle middle `json:"middle"`
		}
		schema := schemaFor[outer]()
		props := schemaProps(t, &schema)

		innerProps := mustNested(t, props, "middle", "properties", "inner", "properties")
		if _, ok := innerProps["value"]; !ok {
			t.Error("expected value in deeply nested struct")
		}
	})
}

func TestSchema_NestedOptional(t *testing.T) {
	t.Parallel()

	t.Run("no_required_when_all_pointers", func(t *testing.T) {
		t.Parallel()
		type inner struct {
			A *string `json:"a"`
			B *int    `json:"b"`
		}
		type outer struct {
			Data inner `json:"data"`
		}
		schema := schemaFor[outer]()
		props := schemaProps(t, &schema)

		dataProp, ok := props["data"].(map[string]any)
		if !ok {
			t.Fatal("expected data property")
		}
		if _, ok := dataProp["required"]; ok {
			t.Error("nested struct with all pointer fields should have no required")
		}
	})

	t.Run("optional_nested_struct", func(t *testing.T) {
		t.Parallel()
		type inner struct {
			Value string `json:"value"`
		}
		type outer struct {
			Data *inner `json:"data"`
		}
		schema := schemaFor[outer]()
		props := schemaProps(t, &schema)

		assertPropType(t, props, "data", "object")
		assertNotRequired(t, &schema, "data")
	})
}

// --- Schema: unexported fields ---

func TestSchema_UnexportedFields(t *testing.T) {
	t.Parallel()

	t.Run("mixed_skips_private", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Public  string `json:"public"`
			private string //nolint:unused // intentionally tests that unexported fields are excluded
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if len(props) != 1 {
			t.Errorf("expected 1 property, got %d", len(props))
		}
	})

	t.Run("all_private_empty", func(t *testing.T) {
		t.Parallel()
		type s struct {
			a string //nolint:unused // intentionally tests all-unexported struct
			b int    //nolint:unused // intentionally tests all-unexported struct
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if len(props) != 0 {
			t.Errorf("expected 0 properties, got %d", len(props))
		}
	})
}

// --- Schema: caching ---

func TestSchema_Caching(t *testing.T) {
	t.Parallel()

	t.Run("same_type_returns_cached", func(t *testing.T) {
		t.Parallel()
		var zero mountError
		typ := reflect.TypeOf(zero)
		schemaCache.Delete(typ)

		s1 := schemaFor[mountError]()
		s2 := schemaFor[mountError]()

		p1 := schemaProps(t, &s1)
		p2 := schemaProps(t, &s2)
		if len(p1) != len(p2) {
			t.Error("cached schema should match")
		}
	})

	t.Run("different_types_different_schemas", func(t *testing.T) {
		t.Parallel()
		type a struct {
			X string `json:"x"`
		}
		type b struct {
			Y int `json:"y"`
		}

		sa := schemaFor[a]()
		sb := schemaFor[b]()
		pa := schemaProps(t, &sa)
		pb := schemaProps(t, &sb)

		if _, ok := pa["x"]; !ok {
			t.Error("expected x in schema a")
		}
		if _, ok := pb["y"]; !ok {
			t.Error("expected y in schema b")
		}
		if _, ok := pa["y"]; ok {
			t.Error("schema a should not have y")
		}
	})
}

// --- Schema: edge cases ---

func TestSchema_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty_struct", func(t *testing.T) {
		t.Parallel()
		type empty struct{}
		schema := schemaFor[empty]()
		props := schemaProps(t, &schema)
		if len(props) != 0 {
			t.Errorf("expected 0 properties for empty struct, got %d", len(props))
		}
		if len(schema.Required) != 0 {
			t.Errorf("expected no required fields, got %v", schema.Required)
		}
	})

	t.Run("single_field", func(t *testing.T) {
		t.Parallel()
		type s struct {
			Only string `json:"only"`
		}
		schema := schemaFor[s]()
		props := schemaProps(t, &schema)
		if len(props) != 1 {
			t.Errorf("expected 1 property, got %d", len(props))
		}
		assertRequired(t, &schema, "only")
	})

	t.Run("many_fields", func(t *testing.T) {
		t.Parallel()
		type wide struct {
			F1  string   `json:"f1"`
			F2  int      `json:"f2"`
			F3  bool     `json:"f3"`
			F4  float64  `json:"f4"`
			F5  string   `json:"f5"`
			F6  *string  `json:"f6"`
			F7  *int     `json:"f7"`
			F8  []string `json:"f8"`
			F9  string   `json:"f9"`
			F10 string   `json:"f10"`
		}

		schema := schemaFor[wide]()
		props := schemaProps(t, &schema)
		if len(props) != 10 {
			t.Errorf("expected 10 properties, got %d", len(props))
		}
		assertRequired(t, &schema, "f1", "f2", "f3", "f4", "f5", "f8", "f9", "f10")
		assertNotRequired(t, &schema, "f6", "f7")
	})
}

// --- Schema: complex real-world structs ---

func TestSchema_HTTPRequest(t *testing.T) {
	t.Parallel()

	type header struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	type httpReq struct {
		Method  string   `json:"method" sense:"HTTP method (GET, POST, etc)"`
		URL     string   `json:"url" sense:"Full request URL"`
		Status  int      `json:"status" sense:"HTTP status code"`
		Headers []header `json:"headers"`
		Body    *string  `json:"body" sense:"Request body if present"`
	}

	schema := schemaFor[httpReq]()
	props := schemaProps(t, &schema)

	assertPropType(t, props, "method", "string")
	assertPropType(t, props, "url", "string")
	assertPropType(t, props, "status", "integer")
	assertPropType(t, props, "headers", "array")
	assertPropType(t, props, "body", "string")

	assertRequired(t, &schema, "method", "url", "status", "headers")
	assertNotRequired(t, &schema, "body")

	headerItems := mustNested(t, props, "headers", "items")
	if headerItems["type"] != "object" {
		t.Errorf("expected object items for headers, got %v", headerItems["type"])
	}

	methodProp, ok := props["method"].(map[string]any)
	if !ok {
		t.Fatal("expected method property")
	}
	if methodProp["description"] != "HTTP method (GET, POST, etc)" {
		t.Errorf("wrong method description: %v", methodProp["description"])
	}
}

func TestSchema_LogEntry(t *testing.T) {
	t.Parallel()

	type logSource struct {
		File     string `json:"file"`
		Line     int    `json:"line"`
		Function string `json:"function"`
	}
	type logEntry struct {
		Timestamp  string    `json:"timestamp"`
		Level      string    `json:"level" sense:"Log level: DEBUG, INFO, WARN, ERROR, FATAL"`
		Message    string    `json:"message"`
		Source     logSource `json:"source"`
		Tags       []string  `json:"tags"`
		ErrorCode  *int      `json:"error_code" sense:"Numeric error code if present"`
		StackTrace *string   `json:"stack_trace"`
	}

	schema := schemaFor[logEntry]()
	props := schemaProps(t, &schema)

	if len(props) != 7 {
		t.Errorf("expected 7 properties, got %d", len(props))
	}
	assertRequired(t, &schema, "timestamp", "level", "message", "source", "tags")
	assertNotRequired(t, &schema, "error_code", "stack_trace")
	assertPropType(t, props, "source", "object")
	assertPropType(t, props, "tags", "array")
}

func TestSchema_DatabaseRecord(t *testing.T) {
	t.Parallel()

	type column struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Nullable bool   `json:"nullable"`
	}
	type index struct {
		Name    string   `json:"name"`
		Columns []string `json:"columns"`
		Unique  bool     `json:"unique"`
	}
	type table struct {
		Schema   string   `json:"schema"`
		Name     string   `json:"name"`
		Columns  []column `json:"columns"`
		Indexes  []index  `json:"indexes"`
		RowCount *int64   `json:"row_count"`
	}

	schema := schemaFor[table]()
	props := schemaProps(t, &schema)

	assertRequired(t, &schema, "schema", "name", "columns", "indexes")
	assertNotRequired(t, &schema, "row_count")

	colProps := mustNested(t, props, "columns", "items", "properties")
	if _, ok := colProps["name"]; !ok {
		t.Error("expected name in column properties")
	}
	if _, ok := colProps["nullable"]; !ok {
		t.Error("expected nullable in column properties")
	}

	idxColsProp := mustNested(t, props, "indexes", "items", "properties", "columns")
	if idxColsProp["type"] != "array" {
		t.Errorf("expected array for index columns, got %v", idxColsProp["type"])
	}
	colsItems, ok := idxColsProp["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items in index columns")
	}
	if colsItems["type"] != "string" {
		t.Errorf("expected string items for index columns, got %v", colsItems["type"])
	}
}

// --- helpers ---

func schemaProps(t *testing.T, schema *anthropic.ToolInputSchemaParam) map[string]any {
	t.Helper()
	props, ok := schema.Properties.(map[string]any)
	if !ok {
		t.Fatalf("expected Properties to be map[string]any, got %T", schema.Properties)
	}
	return props
}

func assertPropType(t *testing.T, props map[string]any, name, expectedType string) {
	t.Helper()
	prop, ok := props[name].(map[string]any)
	if !ok {
		t.Errorf("expected property %s", name)
		return
	}
	if prop["type"] != expectedType {
		t.Errorf("property %s: expected type %s, got %v", name, expectedType, prop["type"])
	}
}

// mustNested navigates a chain of map keys, asserting each step is map[string]any.
func mustNested(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	current := m
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any at key %q, got %T", key, current[key])
		}
		current = next
	}
	return current
}

func assertNotRequired(t *testing.T, schema *anthropic.ToolInputSchemaParam, names ...string) {
	t.Helper()
	for _, name := range names {
		for _, r := range schema.Required {
			if r == name {
				t.Errorf("%s should NOT be required, but is", name)
			}
		}
	}
}

func assertArrayItems(t *testing.T, props map[string]any, name, itemType string) {
	t.Helper()
	prop, ok := props[name].(map[string]any)
	if !ok {
		t.Errorf("expected property %s", name)
		return
	}
	if prop["type"] != "array" {
		t.Errorf("expected %s to be array, got %v", name, prop["type"])
		return
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		t.Errorf("expected items in %s array schema", name)
		return
	}
	if items["type"] != itemType {
		t.Errorf("expected %s items type %s, got %v", name, itemType, items["type"])
	}
}

func assertRequired(t *testing.T, schema *anthropic.ToolInputSchemaParam, names ...string) {
	t.Helper()
	for _, name := range names {
		found := false
		for _, r := range schema.Required {
			if r == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in required, got %v", name, schema.Required)
		}
	}
}
