package slog

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestErrExpandsErrorWithStack(t *testing.T) {
	attr := Err(errors.New("database unavailable"))

	if attr.Key != "error" || attr.Value.Kind() != KindGroup {
		t.Fatalf("Err() = %v, want error group", attr)
	}
	fields := make(map[string]any)
	for _, field := range attr.Value.Group() {
		fields[field.Key] = field.Value.Any()
	}
	if fields["message"] != "database unavailable" {
		t.Fatalf("message = %v", fields["message"])
	}
	if fields["type"] != "*errors.errorString" {
		t.Fatalf("type = %v", fields["type"])
	}
	if !strings.Contains(fields["stacktrace"].(string), "TestErrExpandsErrorWithStack") {
		t.Fatalf("stacktrace = %v", fields["stacktrace"])
	}
	if strings.Contains(fields["stacktrace"].(string), "(*Logger).logRecord") {
		t.Fatalf("stacktrace contains slog internals: %v", fields["stacktrace"])
	}
}

func TestLoggerExpandsErrAttribute(t *testing.T) {
	var output bytes.Buffer
	logger := NewLoggerBuilder().
		WithWriter(&output).
		EnableText(false).
		EnableJSON(true).
		Build()

	logger.Error("request failed", Err(errors.New("database unavailable")))

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	errorFields, ok := record["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %#v, want object", record["error"])
	}
	if errorFields["message"] != "database unavailable" {
		t.Fatalf("message = %v", errorFields["message"])
	}
	if !strings.Contains(errorFields["stacktrace"].(string), "TestLoggerExpandsErrAttribute") {
		t.Fatalf("stacktrace = %v", errorFields["stacktrace"])
	}
}

func TestErr(t *testing.T) {
	err := errors.New("database unavailable")
	attr := Err(err)

	if attr.Key != "error" {
		t.Fatalf("key = %q, want error", attr.Key)
	}
	if attr.Value.Kind() != KindGroup {
		t.Fatalf("value kind = %v, want group", attr.Value.Kind())
	}
}

func TestAnyKeepsDefaultErrorBehavior(t *testing.T) {
	var output bytes.Buffer
	logger := NewLoggerBuilder().
		WithWriter(&output).
		EnableText(false).
		EnableJSON(true).
		Build()

	logger.Error("request failed", Any("error", errors.New("database unavailable")))

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if got, ok := record["error"].(string); !ok || got != "database unavailable" {
		t.Fatalf("error field = %#v, want default error string", record["error"])
	}
}
