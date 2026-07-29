package dlp

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type testLogger struct {
	debugMessages []string
}

func (*testLogger) Error(string, ...any) {}
func (*testLogger) Warn(string, ...any)  {}
func (l *testLogger) Debug(message string, _ ...any) {
	l.debugMessages = append(l.debugMessages, message)
}

func TestBaseDesensitizerConfigurationAndState(t *testing.T) {
	base := NewBaseDesensitizer("token")
	logger := &testLogger{}
	base.SetLogger(logger)
	if base.Name() != "token" || !base.Enabled() || !base.CacheEnabled() {
		t.Fatalf("initial state = name %q, enabled %v, cache %v", base.Name(), base.Enabled(), base.CacheEnabled())
	}

	base.Disable()
	base.Enable()
	if !base.Enabled() || len(logger.debugMessages) != 2 {
		t.Fatalf("state transitions = enabled %v, messages %v", base.Enabled(), logger.debugMessages)
	}
	if err := base.Configure(nil); err == nil {
		t.Fatal("Configure(nil) error = nil")
	}
	config := map[string]any{"cache_enabled": false, "prefix": "secret"}
	if err := base.Configure(config); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	config["prefix"] = "changed"
	if got, ok := base.GetConfig("prefix"); !ok || got != "secret" {
		t.Fatalf("copied config prefix = %v, %v", got, ok)
	}
	if base.CacheEnabled() {
		t.Fatal("cache_enabled=false was not applied")
	}
	if _, ok := base.GetConfig("missing"); ok {
		t.Fatal("GetConfig() found a missing key")
	}
}

func TestRegexDesensitizerPatternsCacheAndDisable(t *testing.T) {
	desensitizer := NewRegexDesensitizer("credentials")
	if err := desensitizer.AddPattern("token", `token=[a-z]+`, "token=***"); err != nil {
		t.Fatalf("AddPattern() error = %v", err)
	}
	if err := desensitizer.AddPattern("invalid", `[`, "x"); err == nil {
		t.Fatal("AddPattern(invalid) error = nil")
	}
	if !desensitizer.Supports("token") || desensitizer.Supports("missing") {
		t.Fatal("Supports() returned an unexpected result")
	}
	if desensitizer.GetTypePattern("token") != `token=[a-z]+` || desensitizer.GetTypePattern("missing") != "" {
		t.Fatal("GetTypePattern() returned an unexpected pattern")
	}
	if !slices.Contains(desensitizer.GetSupportedTypes(), "token") {
		t.Fatalf("supported types = %v", desensitizer.GetSupportedTypes())
	}
	if !desensitizer.ValidateType("token=secret", "token") || desensitizer.ValidateType("plain", "missing") {
		t.Fatal("ValidateType() returned an unexpected result")
	}

	for i := 0; i < 2; i++ {
		got, err := desensitizer.Desensitize("token=secret")
		if err != nil || got != "token=***" {
			t.Fatalf("Desensitize() = %q, %v", got, err)
		}
	}
	stats := desensitizer.GetCacheStats()
	if stats.Misses != 1 || stats.Hits != 1 || stats.Size != 1 || stats.HitRatio != 0.5 {
		t.Fatalf("cache stats = %+v", stats)
	}

	desensitizer.SetCacheEnabled(false)
	if desensitizer.CacheEnabled() || desensitizer.GetCacheStats() != (CacheStats{}) {
		t.Fatalf("cache was not cleared: %+v", desensitizer.GetCacheStats())
	}
	desensitizer.Disable()
	if got, err := desensitizer.Desensitize("token=secret"); err != nil || got != "token=secret" {
		t.Fatalf("disabled Desensitize() = %q, %v", got, err)
	}
}

func TestRegexDesensitizerContextAndBatch(t *testing.T) {
	desensitizer := NewRegexDesensitizer("digits")
	if err := desensitizer.AddPattern("digits", `\d+`, "*"); err != nil {
		t.Fatalf("AddPattern() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := desensitizer.DesensitizeWithContext(ctx, "id=123"); !errors.Is(err, context.Canceled) || got != "id=123" {
		t.Fatalf("canceled DesensitizeWithContext() = %q, %v", got, err)
	}
	got, err := desensitizer.BatchDesensitize([]string{"id=1", "id=22"})
	if err != nil || !slices.Equal(got, []string{"id=*", "id=*"}) {
		t.Fatalf("BatchDesensitize() = %v, %v", got, err)
	}

	desensitizer.Disable()
	input := []string{"id=1"}
	got, err = desensitizer.BatchDesensitize(input)
	if err != nil || !slices.Equal(got, input) {
		t.Fatalf("disabled BatchDesensitize() = %v, %v", got, err)
	}
}

func TestCustomFunctionDesensitizerUsesFirstMatchingFunction(t *testing.T) {
	desensitizer := NewCustomFunctionDesensitizer("custom")
	desensitizer.AddFunction("email", func(value string) string {
		if value == "alice@example.com" {
			return "a***@example.com"
		}
		return value
	})
	if !desensitizer.Supports("email") || desensitizer.Supports("phone") {
		t.Fatal("Supports() returned an unexpected result")
	}
	if !slices.Contains(desensitizer.GetSupportedTypes(), "email") {
		t.Fatalf("supported types = %v", desensitizer.GetSupportedTypes())
	}
	if got, err := desensitizer.Desensitize("alice@example.com"); err != nil || got != "a***@example.com" {
		t.Fatalf("Desensitize() = %q, %v", got, err)
	}
	if got, err := desensitizer.Desensitize("plain"); err != nil || got != "plain" {
		t.Fatalf("unmatched Desensitize() = %q, %v", got, err)
	}
	desensitizer.Disable()
	if got, _ := desensitizer.Desensitize("alice@example.com"); got != "alice@example.com" {
		t.Fatalf("disabled Desensitize() = %q", got)
	}
}
