package sense

import (
	"encoding/json"
	"errors"
	"testing"
)

// --- ExtractSlice behavior tests ---

func TestExtractSlice_EmptySlice(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"items": []}`),
		usage:    &Usage{InputTokens: 100, OutputTokens: 20},
	}
	s := testSession(mock)

	result, err := newExtractSliceBuilder[mountError](s, "no errors here").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result.Data))
	}
	if result.TokensUsed != 120 {
		t.Errorf("expected 120 tokens, got %d", result.TokensUsed)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestExtractSlice_SingleItem(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"items": [
				{"device": "/dev/sdf", "volume_id": "vol-123", "message": "already mounted"}
			]
		}`),
		usage: &Usage{InputTokens: 200, OutputTokens: 60},
	}
	s := testSession(mock)

	result, err := newExtractSliceBuilder[mountError](s, "device /dev/sdf already mounted with vol-123").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Data))
	}
	if result.Data[0].Device != "/dev/sdf" {
		t.Errorf("expected /dev/sdf, got %s", result.Data[0].Device)
	}
	if result.Data[0].VolumeID != "vol-123" {
		t.Errorf("expected vol-123, got %s", result.Data[0].VolumeID)
	}
	if result.TokensUsed != 260 {
		t.Errorf("expected 260 tokens, got %d", result.TokensUsed)
	}
}

func TestExtractSlice_MultipleItems(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"items": [
				{"device": "/dev/sdf", "volume_id": "vol-111", "message": "mounted"},
				{"device": "/dev/sdg", "volume_id": "vol-222", "message": "busy"},
				{"device": "/dev/sdh", "volume_id": "vol-333", "message": "timeout"}
			]
		}`),
		usage: &Usage{InputTokens: 300, OutputTokens: 120},
	}
	s := testSession(mock)

	result, err := newExtractSliceBuilder[mountError](s, "three errors in log").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Data))
	}
	if result.Data[0].Device != "/dev/sdf" {
		t.Errorf("item 0: expected /dev/sdf, got %s", result.Data[0].Device)
	}
	if result.Data[1].Device != "/dev/sdg" {
		t.Errorf("item 1: expected /dev/sdg, got %s", result.Data[1].Device)
	}
	if result.Data[2].Device != "/dev/sdh" {
		t.Errorf("item 2: expected /dev/sdh, got %s", result.Data[2].Device)
	}
}

func TestExtractSlice_NestedStructs(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"items": [
				{"name": "Alice", "age": 30, "active": true, "address": {"city": "Portland", "state": "OR"}},
				{"name": "Bob", "age": 25, "active": false, "address": {"city": "Seattle", "state": "WA"}}
			]
		}`),
		usage: &Usage{InputTokens: 200, OutputTokens: 100},
	}
	s := testSession(mock)

	result, err := newExtractSliceBuilder[personRecord](s, "Alice 30 Portland OR, Bob 25 Seattle WA").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Data))
	}
	if result.Data[0].Name != "Alice" {
		t.Errorf("expected Alice, got %s", result.Data[0].Name)
	}
	if result.Data[0].Address.City != "Portland" {
		t.Errorf("expected Portland, got %s", result.Data[0].Address.City)
	}
	if result.Data[1].Name != "Bob" {
		t.Errorf("expected Bob, got %s", result.Data[1].Name)
	}
	if result.Data[1].Address.State != "WA" {
		t.Errorf("expected WA, got %s", result.Data[1].Address.State)
	}
}

func TestExtractSlice_OptionalFields(t *testing.T) {
	email := "alice@example.com"
	mock := &mockCaller{
		response: json.RawMessage(`{
			"items": [
				{"name": "Alice", "email": "alice@example.com", "phone": null},
				{"name": "Bob", "email": null, "phone": null}
			]
		}`),
		usage: &Usage{InputTokens: 150, OutputTokens: 60},
	}
	s := testSession(mock)

	result, err := newExtractSliceBuilder[optionalFields](s, "Alice alice@example.com, Bob").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Data))
	}
	if result.Data[0].Email == nil || *result.Data[0].Email != email {
		t.Errorf("expected email=%s, got %v", email, result.Data[0].Email)
	}
	if result.Data[0].Phone != nil {
		t.Errorf("expected nil phone for Alice, got %v", result.Data[0].Phone)
	}
	if result.Data[1].Email != nil {
		t.Errorf("expected nil email for Bob, got %v", result.Data[1].Email)
	}
}

func TestExtractSlice_EmptyText(t *testing.T) {
	s := testSession(&mockCaller{})

	_, err := newExtractSliceBuilder[mountError](s, "").Run()
	if !errors.Is(err, ErrNoText) {
		t.Errorf("expected ErrNoText, got %v", err)
	}
}

func TestExtractSlice_ClientError(t *testing.T) {
	mock := &mockCaller{err: errors.New("connection refused")}
	s := testSession(mock)

	_, err := newExtractSliceBuilder[mountError](s, "some text").Run()
	if err == nil {
		t.Fatal("expected error")
	}
	var senseErr *Error
	if !errors.As(err, &senseErr) {
		t.Fatalf("expected *sense.Error, got %T", err)
	}
	if senseErr.Op != "extract_slice" {
		t.Errorf("expected op=extract_slice, got %s", senseErr.Op)
	}
}

func TestExtractSlice_BadJSON(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{not valid}`),
		usage:    &Usage{InputTokens: 100, OutputTokens: 30},
	}
	s := testSession(mock)

	_, err := newExtractSliceBuilder[mountError](s, "some text").Run()
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}

	u := s.Usage()
	if u.Calls != 1 {
		t.Errorf("expected 1 call recorded, got %d", u.Calls)
	}
}

func TestExtractSlice_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")
	s := testSession(&mockCaller{})

	result, err := newExtractSliceBuilder[mountError](s, "anything").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data == nil {
		t.Fatal("expected non-nil empty slice in skip mode")
	}
	if len(result.Data) != 0 {
		t.Errorf("expected empty slice in skip mode, got %d items", len(result.Data))
	}
}

func TestExtractSlice_ContextInPrompt(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"items": []}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	_, _ = newExtractSliceBuilder[mountError](s, "some errors").
		Context("AWS EC2 EBS mount errors").
		Run()

	if mock.lastReq == nil {
		t.Fatal("expected a call")
	}
	msg := mock.lastReq.userMessage
	if !contains(msg, "AWS EC2 EBS mount errors") {
		t.Errorf("prompt missing context:\n%s", msg)
	}
	if !contains(msg, "some errors") {
		t.Errorf("prompt missing text:\n%s", msg)
	}
	if mock.lastReq.toolName != "submit_extraction" {
		t.Errorf("expected tool name submit_extraction, got %s", mock.lastReq.toolName)
	}
}

func TestExtractSlice_ModelOverride(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"items": []}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	result, _ := newExtractSliceBuilder[mountError](s, "text").
		Model("claude-haiku-4-5-20251001").
		Run()

	if mock.lastReq.model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected model override, got %s", mock.lastReq.model)
	}
	if result.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected model in result, got %s", result.Model)
	}
}

func TestExtractSlice_RecordsUsage(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"items": []}`),
		usage:    &Usage{InputTokens: 300, OutputTokens: 100},
	}
	s := testSession(mock)

	_, _ = newExtractSliceBuilder[mountError](s, "text").Run()

	u := s.Usage()
	if u.Calls != 1 {
		t.Errorf("expected 1 call, got %d", u.Calls)
	}
	if u.InputTokens != 300 {
		t.Errorf("expected 300 input tokens, got %d", u.InputTokens)
	}
}

func TestExtractSlice_ConcurrentUsage(t *testing.T) {
	mock := &concurrentMockCaller{
		response: json.RawMessage(`{"items": [{"device":"x","volume_id":"y","message":"z"}]}`),
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
			_, _ = newExtractSliceBuilder[mountError](s, "text").Run()
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

// --- Validate ---

func TestExtractSlice_ValidatePass(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"items": [
				{"device": "/dev/sdf", "volume_id": "vol-123", "message": "error"}
			]
		}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 50},
	}
	s := testSession(mock)

	result, err := newExtractSliceBuilder[mountError](s, "text").
		Validate(func(m mountError) error {
			if m.Device == "" {
				return errors.New("device required")
			}
			return nil
		}).
		Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(result.Data))
	}
}

func TestExtractSlice_ValidateFail(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"items": [
				{"device": "/dev/sdf", "volume_id": "vol-123", "message": "ok"},
				{"device": "", "volume_id": "vol-456", "message": "bad"}
			]
		}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 50},
	}
	s := testSession(mock)

	_, err := newExtractSliceBuilder[mountError](s, "text").
		Validate(func(m mountError) error {
			if m.Device == "" {
				return errors.New("device required")
			}
			return nil
		}).
		Run()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var senseErr *Error
	if !errors.As(err, &senseErr) {
		t.Fatalf("expected *sense.Error, got %T", err)
	}
	if senseErr.Op != "extract_slice" {
		t.Errorf("expected op=extract_slice, got %s", senseErr.Op)
	}
	if !contains(senseErr.Message, "item 1") {
		t.Errorf("expected message to reference item 1, got %s", senseErr.Message)
	}
}

// --- Schema: wrapping ---

func TestExtractSlice_SchemaWrapping(t *testing.T) {
	t.Parallel()

	schema := sliceSchemaFor[mountError]()
	props, ok := schema.Properties.(map[string]any)
	if !ok {
		t.Fatalf("expected Properties to be map[string]any, got %T", schema.Properties)
	}

	// Top level should have only "items"
	if len(props) != 1 {
		t.Errorf("expected 1 top-level property, got %d", len(props))
	}

	items, ok := props["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items property")
	}
	if items["type"] != "array" {
		t.Errorf("expected items type=array, got %v", items["type"])
	}

	// Items schema should be the struct schema
	itemSchema, ok := items["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items.items to be object schema")
	}
	if itemSchema["type"] != "object" {
		t.Errorf("expected item type=object, got %v", itemSchema["type"])
	}

	innerProps, ok := itemSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected inner properties")
	}
	if _, ok := innerProps["device"]; !ok {
		t.Error("expected device in item properties")
	}
	if _, ok := innerProps["volume_id"]; !ok {
		t.Error("expected volume_id in item properties")
	}

	// Required should include "items"
	if len(schema.Required) != 1 || schema.Required[0] != "items" {
		t.Errorf("expected required=[items], got %v", schema.Required)
	}
}

func TestExtractSlice_SchemaSenseTagsPreserved(t *testing.T) {
	t.Parallel()

	schema := sliceSchemaFor[mountError]()
	props := schemaProps(t, &schema)
	innerProps := mustNested(t, props, "items", "items", "properties")

	deviceProp, ok := innerProps["device"].(map[string]any)
	if !ok {
		t.Fatal("expected device property")
	}
	desc, ok := deviceProp["description"].(string)
	if !ok || desc != "The device path" {
		t.Errorf("expected sense description preserved, got %v", desc)
	}
}
