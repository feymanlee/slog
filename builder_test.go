package slog

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	gelfmod "github.com/feymanlee/slog/modules/output/gelf"
	outputnet "github.com/feymanlee/slog/modules/output/net"
)

type builderContextKey string

const traceIDBuilderContextKey builderContextKey = "trace_id"

func TestLoggerBuilder_BuildsLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLoggerBuilder().
		WithWriter(buf).
		WithModule("order").
		WithGroup("api").
		WithAttrs(String("req_id", "r1")).
		EnableJSON(false).
		EnableText(true).
		Build()

	logger.Info("ok")
	out := buf.String()
	if !strings.Contains(out, "module=order") {
		t.Fatalf("expected module field, got %s", out)
	}
	if !strings.Contains(out, "api.req_id=r1") {
		t.Fatalf("expected grouped attr, got %s", out)
	}
}

func TestLoggerBuilder_ContextHelper(t *testing.T) {
	SetContextPropagator(func(ctx context.Context) []Attr {
		if v, ok := ctx.Value(traceIDBuilderContextKey).(string); ok {
			return []Attr{String("trace_id", v)}
		}
		return nil
	})
	t.Cleanup(func() { SetContextPropagator(nil) })

	buf := &bytes.Buffer{}
	logger := NewLoggerBuilder().WithWriter(buf).Build()
	ctx := context.WithValue(context.Background(), traceIDBuilderContextKey, "abc-123")
	logger.InfoContext(ctx, "ctx message")

	out := buf.String()
	if !strings.Contains(out, "trace_id=abc-123") {
		t.Fatalf("expected propagated trace_id, got %s", out)
	}
}

func TestLoggerBuilder_LogfmtMode(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLoggerBuilder().WithWriter(buf).UseLogfmt().Build()
	logger.Info("lfmt", String("k", "v"))
	out := buf.String()
	if !strings.Contains(out, "k=v") || strings.Contains(out, "{") {
		t.Fatalf("logfmt output malformed: %s", out)
	}
}

func TestLoggerBuilderCopiesConfigOutputFlags(t *testing.T) {
	config := DefaultConfig()
	config.SetEnableText(true)
	config.SetEnableJSON(false)
	builder := NewLoggerBuilder().WithConfig(config)

	*config.EnableText = false
	*config.EnableJSON = true
	var buf bytes.Buffer
	logger := builder.WithWriter(&buf).Build()
	logger.Info("copied config")

	if out := buf.String(); !strings.Contains(out, "copied config") || strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("builder retained external config pointers, output = %q", out)
	}
}

func TestLoggerBuilderGELFMode(t *testing.T) {
	var buf bytes.Buffer
	options := &gelfmod.Options{Host: "builder-host", Facility: "audit"}
	logger := NewLoggerBuilder().
		WithWriter(&buf).
		WithAttrs(String("request_id", "r1")).
		UseGELF(options).
		Build()
	logger.Info("gelf event")

	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload); err != nil {
		t.Fatalf("GELF output is not JSON: %v; output=%q", err, buf.String())
	}
	if payload["version"] != "1.1" || payload["host"] != "builder-host" || payload["facility"] != "audit" {
		t.Fatalf("GELF metadata = %#v", payload)
	}
	if payload["short_message"] != "gelf event" || payload["_request_id"] != "r1" {
		t.Fatalf("GELF record = %#v", payload)
	}
}

func TestLoggerBuilderNetOutputMode(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	readDone := make(chan string, 1)
	go func() {
		buffer := make([]byte, 256)
		n, err := server.Read(buffer)
		if err == nil {
			readDone <- string(buffer[:n])
		}
	}()

	options := &outputnet.SenderOption{
		Network:      "tcp",
		Addr:         "pipe",
		WriteTimeout: time.Second,
		Delimiter:    []byte("\n"),
		Dial: func(string, string, time.Duration) (net.Conn, error) {
			return client, nil
		},
	}
	logger := NewLoggerBuilder().
		UseNetOutput(options).
		WithAttrs(String("request_id", "r1")).
		Build()
	logger.Info("network event")

	select {
	case payload := <-readDone:
		if payload != "level=INFO msg=network event request_id=r1\n" {
			t.Fatalf("network payload = %q", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for builder network output")
	}
}
