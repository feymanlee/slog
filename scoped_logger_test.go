package slog

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/darkit/slog/modules"
	"github.com/darkit/slog/modules/formatter"
)

func TestLoggerSetLevelDoesNotAffectOtherLoggers(t *testing.T) {
	resetForTest()
	SetLevelInfo()
	EnableTextLogger()
	DisableJSONLogger()

	var firstBuf bytes.Buffer
	var secondBuf bytes.Buffer
	first := NewLogger(&firstBuf, true, false)
	second := NewLogger(&secondBuf, true, false)

	first.SetLevel(LevelError)

	first.Info("first info")
	second.Info("second info")

	if strings.Contains(firstBuf.String(), "first info") {
		t.Fatalf("expected first logger info to be filtered, got %q", firstBuf.String())
	}
	if !strings.Contains(secondBuf.String(), "second info") {
		t.Fatalf("expected second logger to keep global info level, got %q", secondBuf.String())
	}
}

func TestLoggerUseFormatterDoesNotAffectOtherLoggers(t *testing.T) {
	resetForTest()
	SetLevelInfo()
	EnableTextLogger()
	DisableJSONLogger()

	var formattedBuf bytes.Buffer
	var plainBuf bytes.Buffer
	formatted := NewLogger(&formattedBuf, true, false)
	plain := NewLogger(&plainBuf, true, false)

	module := formatter.NewFormatterAdapter()
	if err := module.Configure(modules.Config{
		"type":        "error",
		"replacement": "error",
	}); err != nil {
		t.Fatalf("configure formatter: %v", err)
	}
	formatted.Use(module)

	err := errors.New("boom")
	formatted.Error("formatted", "error", err)
	plain.Error("plain", "error", err)

	if !strings.Contains(formattedBuf.String(), "error.message=boom") {
		t.Fatalf("expected formatter on selected logger, got %q", formattedBuf.String())
	}
	if strings.Contains(plainBuf.String(), "error.message=boom") {
		t.Fatalf("expected plain logger to remain unformatted, got %q", plainBuf.String())
	}
	if !strings.Contains(plainBuf.String(), "error=boom") {
		t.Fatalf("expected plain logger to keep original error attr, got %q", plainBuf.String())
	}
}

func TestLoggerBuilderEnableDLPDoesNotAffectOtherLoggers(t *testing.T) {
	resetForTest()
	SetLevelInfo()
	EnableTextLogger()
	DisableJSONLogger()
	DisableDLPLogger()

	var dlpBuf bytes.Buffer
	var plainBuf bytes.Buffer

	dlpLogger := NewLoggerBuilder().
		WithWriter(&dlpBuf).
		EnableText(true).
		EnableJSON(false).
		EnableDLP(true).
		Build()
	plainLogger := NewLoggerBuilder().
		WithWriter(&plainBuf).
		EnableText(true).
		EnableJSON(false).
		Build()

	dlpLogger.Info("login", "phone", "13812345678")
	plainLogger.Info("login", "phone", "13812345678")

	if strings.Contains(dlpBuf.String(), "13812345678") {
		t.Fatalf("expected builder DLP logger to mask phone, got %q", dlpBuf.String())
	}
	if !strings.Contains(plainBuf.String(), "13812345678") {
		t.Fatalf("expected plain builder logger to remain unmasked, got %q", plainBuf.String())
	}
	if IsDLPEnabled() {
		t.Fatal("expected builder-scoped DLP not to flip global DLP state")
	}
}

func TestGlobalDLPStillAffectsExistingUnscopedLogger(t *testing.T) {
	resetForTest()
	SetLevelInfo()
	EnableTextLogger()
	DisableJSONLogger()
	DisableDLPLogger()
	defer DisableDLPLogger()

	var buf bytes.Buffer
	logger := NewLogger(&buf, true, false)

	EnableDLPLogger()
	logger.Info("login", "phone", "13812345678")

	if strings.Contains(buf.String(), "13812345678") {
		t.Fatalf("expected global DLP to affect existing unscoped logger, got %q", buf.String())
	}
}

func TestSetDefaultLoggerStillFollowsGlobalLevel(t *testing.T) {
	resetForTest()
	SetLevelInfo()
	EnableTextLogger()
	DisableJSONLogger()

	var buf bytes.Buffer
	logger := NewLoggerWithConfig(&buf, DefaultConfig())
	SetDefault(logger)
	t.Cleanup(func() {
		ResetGlobalLogger(&bytes.Buffer{}, true, false)
		SetLevelInfo()
	})

	SetLevelError()
	Info("filtered info")

	if strings.Contains(buf.String(), "filtered info") {
		t.Fatalf("expected global SetLevel to filter default logger output, got %q", buf.String())
	}
}

func TestSetDefaultScopedLoggerStillFollowsGlobalLevel(t *testing.T) {
	resetForTest()
	SetLevelInfo()
	EnableTextLogger()
	DisableJSONLogger()

	var buf bytes.Buffer
	logger := NewLoggerWithConfig(&buf, DefaultConfig())
	logger.SetLevel(LevelTrace)
	SetDefault(logger)
	t.Cleanup(func() {
		ResetGlobalLogger(&bytes.Buffer{}, true, false)
		SetLevelInfo()
	})

	SetLevelError()
	Info("filtered scoped info")

	if strings.Contains(buf.String(), "filtered scoped info") {
		t.Fatalf("expected global SetLevel to filter scoped default logger output, got %q", buf.String())
	}
}
