package slog

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestErrAttrExpandsErrorWithStack(t *testing.T) {
	attr := errAttr(Any("error", errors.New("database unavailable")))

	if attr.Key != "error" || attr.Value.Kind() != KindGroup {
		t.Fatalf("errAttr() = %v, want error group", attr)
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
	if !strings.Contains(fields["stacktrace"].(string), "TestErrAttrExpandsErrorWithStack") {
		t.Fatalf("stacktrace = %v", fields["stacktrace"])
	}
	if strings.Contains(fields["stacktrace"].(string), "(*Logger).logRecord") {
		t.Fatalf("stacktrace contains slog internals: %v", fields["stacktrace"])
	}
}

func TestLoggerExpandsErrorAttribute(t *testing.T) {
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
	errorFields, ok := record["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %#v, want object", record["error"])
	}
	if errorFields["message"] != "database unavailable" {
		t.Fatalf("message = %v", errorFields["message"])
	}
	if !strings.Contains(errorFields["stacktrace"].(string), "TestLoggerExpandsErrorAttribute") {
		t.Fatalf("stacktrace = %v", errorFields["stacktrace"])
	}
}

func TestErrAttrKeepsNilAndNonErrorValues(t *testing.T) {
	nilAttr := Any("error", nil)
	if got := errAttr(nilAttr); got.Value.Kind() != nilAttr.Value.Kind() {
		t.Fatalf("nil error attr changed: %v", got)
	}
	plainAttr := String("error", "already formatted")
	if got := errAttr(plainAttr); got.Value.String() != plainAttr.Value.String() {
		t.Fatalf("plain error attr changed: %v", got)
	}
}
