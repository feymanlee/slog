package common

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"
)

type textValue struct {
	value string
	err   error
}

func (v textValue) MarshalText() ([]byte, error) { return []byte(v.value), v.err }

type logValue string

func (v logValue) LogValue() slog.Value { return slog.StringValue(string(v)) }

func TestAppendRecordAttrsWrapsRecordGroupsWithoutMutatingBase(t *testing.T) {
	base := []slog.Attr{slog.String("service", "api")}
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "message", 0)
	record.AddAttrs(slog.String("request_id", "r1"))

	got := AppendRecordAttrsToAttrs(base, []string{"http", "request"}, &record)
	if len(base) != 1 || len(got) != 2 {
		t.Fatalf("lengths = base %d, output %d", len(base), len(got))
	}
	httpGroup := got[1]
	if httpGroup.Key != "http" || httpGroup.Value.Kind() != slog.KindGroup {
		t.Fatalf("outer attr = %v", httpGroup)
	}
	requestGroup := httpGroup.Value.Group()[0]
	if requestGroup.Key != "request" || requestGroup.Value.Group()[0].Key != "request_id" {
		t.Fatalf("nested attr = %v", requestGroup)
	}
}

func TestReplaceAttrsVisitsLeavesWithTheirGroupPath(t *testing.T) {
	var paths []string
	attrs := []slog.Attr{
		slog.String("root", "value"),
		slog.Group("http", slog.Group("request", slog.String("id", "r1"))),
	}
	got := ReplaceAttrs(func(groups []string, attr slog.Attr) slog.Attr {
		paths = append(paths, strings.Join(append(append([]string(nil), groups...), attr.Key), "."))
		return slog.String(attr.Key, strings.ToUpper(attr.Value.String()))
	}, nil, attrs...)

	if strings.Join(paths, ",") != "root,http.request.id" {
		t.Fatalf("visited paths = %v", paths)
	}
	nested, ok := FindAttribute(got, []string{"http", "request"}, "id")
	if !ok || nested.Value.String() != "R1" {
		t.Fatalf("replaced nested attr = %v, %v", nested, ok)
	}
}

func TestAttrsToMapMergesDuplicateGroupsAndUsesLastScalar(t *testing.T) {
	got := AttrsToMap(
		slog.Group("request", slog.String("id", "r1")),
		slog.Group("request", slog.Int("attempt", 2)),
		slog.String("status", "queued"),
		slog.String("status", "sent"),
	)
	request, ok := got["request"].(map[string]any)
	if !ok || request["id"] != "r1" || request["attempt"] != int64(2) {
		t.Fatalf("merged request = %#v", got["request"])
	}
	if got["status"] != "sent" {
		t.Fatalf("status = %v, want last value", got["status"])
	}
}

func TestAttrToValueSupportsStructuredKinds(t *testing.T) {
	now := time.Date(2026, 7, 29, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		attr slog.Attr
		want any
	}{
		{attr: slog.Any("any", []string{"a"}), want: []string{"a"}},
		{attr: slog.Any("valuer", logValue("resolved")), want: "resolved"},
		{attr: slog.Group("group", slog.String("key", "value")), want: map[string]any{"key": "value"}},
		{attr: slog.Int64("int", -2), want: int64(-2)},
		{attr: slog.Uint64("uint", 2), want: uint64(2)},
		{attr: slog.Float64("float", 1.5), want: 1.5},
		{attr: slog.String("string", "value"), want: "value"},
		{attr: slog.Bool("bool", true), want: true},
		{attr: slog.Duration("duration", time.Second), want: time.Second},
		{attr: slog.Time("time", now), want: now.UTC()},
	}
	for _, test := range tests {
		t.Run(test.attr.Key, func(t *testing.T) {
			key, got := AttrToValue(test.attr)
			if key != test.attr.Key || fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("AttrToValue() = %q, %#v, want %#v", key, got, test.want)
			}
		})
	}
}

func TestValueToStringAndTextMarshaler(t *testing.T) {
	if got := AnyValueToString(slog.AnyValue(textValue{value: "encoded"})); got != "encoded" {
		t.Fatalf("AnyValueToString(text marshaler) = %q", got)
	}
	if got := AnyValueToString(slog.AnyValue(textValue{err: errors.New("encode")})); got != "" {
		t.Fatalf("AnyValueToString(failing marshaler) = %q", got)
	}

	now := time.Unix(100, 0)
	attrs := []slog.Attr{
		slog.Any("any", 12),
		slog.Int64("int", -2),
		slog.Uint64("uint", 2),
		slog.Float64("float", 1.5),
		slog.String("string", "value"),
		slog.Bool("bool", true),
		slog.Duration("duration", time.Second),
		slog.Time("time", now),
	}
	got := AttrsToString(attrs...)
	wants := map[string]string{
		"any": "12", "int": "-2", "uint": "2", "float": "1.500000",
		"string": "value", "bool": "true", "duration": "1s", "time": now.UTC().String(),
	}
	for key, want := range wants {
		if got[key] != want {
			t.Fatalf("AttrsToString()[%q] = %q, want %q", key, got[key], want)
		}
	}
}

func TestErrorAttributeHelpers(t *testing.T) {
	target := errors.New("database unavailable")
	attrs := []slog.Attr{slog.String("request_id", "r1"), slog.Any("error", target)}

	replaced := ReplaceError(append([]slog.Attr(nil), attrs...), "error")
	formatted, ok := replaced[1].Value.Any().(map[string]any)
	if !ok || formatted["error"] != target.Error() || formatted["kind"] != "*errors.errorString" || formatted["stack"] == "" {
		t.Fatalf("formatted error = %#v", replaced[1].Value.Any())
	}

	remaining, extracted := ExtractError(append([]slog.Attr(nil), attrs...), "error")
	if !errors.Is(extracted, target) || len(remaining) != 1 || remaining[0].Key != "request_id" {
		t.Fatalf("ExtractError() = %v, %v", remaining, extracted)
	}
	values := FormatErrorKey(map[string]any{"error": target}, "error")
	if _, ok := values["error"].(map[string]any); !ok {
		t.Fatalf("FormatErrorKey() = %#v", values)
	}
}

func TestFormatRequestCapturesURLQueryAndOptionalHeaders(t *testing.T) {
	requestURL, err := url.Parse("https://api.example.test/items?q=one&q=two#result")
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	request := &http.Request{
		Method: "GET",
		Host:   "proxy.example.test",
		URL:    requestURL,
		Header: http.Header{"X-Request-Id": []string{"r1", "r2"}},
	}

	got := FormatRequest(request, false)
	urlValues := got["url"].(map[string]any)
	if got["host"] != request.Host || got["method"] != "GET" || urlValues["path"] != "/items" {
		t.Fatalf("formatted request = %#v", got)
	}
	if urlValues["query"].(map[string]string)["q"] != "one,two" {
		t.Fatalf("formatted query = %#v", urlValues["query"])
	}
	if got["headers"].(map[string]string)["X-Request-Id"] != "r1,r2" {
		t.Fatalf("formatted headers = %#v", got["headers"])
	}
	if _, ok := FormatRequest(request, true)["headers"]; ok {
		t.Fatal("headers were included when ignoreHeaders=true")
	}
}

func TestSourceHelpersAndAttributeLookup(t *testing.T) {
	pcs := make([]uintptr, 1)
	if runtime.Callers(1, pcs) == 0 {
		t.Fatal("runtime.Callers returned no frame")
	}
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "message", pcs[0])

	source := Source("source", &record)
	values := AttrsToMap(source)["source"].(map[string]any)
	if values["file"] == "" || values["function"] == "" || values["line"].(int64) == 0 {
		t.Fatalf("Source() = %#v", values)
	}
	if got := StringSource("source", &record).Value.String(); !strings.Contains(got, "attributes_test.go") {
		t.Fatalf("StringSource() = %q", got)
	}

	attrs := []slog.Attr{slog.Group("request", slog.String("id", "r1"))}
	if attr, ok := FindAttribute(attrs, []string{"request"}, "id"); !ok || attr.Value.String() != "r1" {
		t.Fatalf("FindAttribute() = %v, %v", attr, ok)
	}
	if _, ok := FindAttribute(attrs, []string{"missing"}, "id"); ok {
		t.Fatal("FindAttribute() found a missing group")
	}
}

func TestRemoveEmptyAttrsRecursivelyDropsEmptyValuesAndGroups(t *testing.T) {
	attrs := []slog.Attr{
		{},
		slog.String("kept", ""),
		slog.Group("empty_group", slog.Attr{}),
		slog.Group("request", slog.Attr{}, slog.String("id", "r1")),
	}
	got := RemoveEmptyAttrs(attrs)
	if len(got) != 2 || got[0].Key != "kept" || got[1].Key != "request" {
		t.Fatalf("RemoveEmptyAttrs() = %v", got)
	}
	if nested := got[1].Value.Group(); len(nested) != 1 || nested[0].Key != "id" {
		t.Fatalf("nested attrs = %v", nested)
	}
}
