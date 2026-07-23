package slog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdslog "log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/darkit/slog/modules"
)

type configurableTestFormatter struct {
	*modules.BaseModule
	mu     sync.RWMutex
	key    string
	prefix string
	err    error
}

func newConfigurableTestFormatter(name, key, prefix string) *configurableTestFormatter {
	return &configurableTestFormatter{
		BaseModule: modules.NewBaseModule(name, modules.TypeFormatter, 10),
		key:        key,
		prefix:     prefix,
	}
}

func (m *configurableTestFormatter) Configure(config modules.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	var cfg struct {
		Key    string `json:"key"`
		Prefix string `json:"prefix"`
	}
	if err := config.Bind(&cfg); err != nil {
		return err
	}
	m.key = cfg.Key
	m.prefix = cfg.Prefix
	return nil
}

func (m *configurableTestFormatter) FormatterFunctions() []func([]string, stdslog.Attr) (stdslog.Value, bool) {
	m.mu.RLock()
	key, prefix := m.key, m.prefix
	m.mu.RUnlock()
	return []func([]string, stdslog.Attr) (stdslog.Value, bool){
		func(_ []string, attr stdslog.Attr) (stdslog.Value, bool) {
			if attr.Key != key {
				return attr.Value, false
			}
			return stdslog.StringValue(prefix + attr.Value.String()), true
		},
	}
}

type handlerTypeFormatterProvider struct {
	*modules.BaseModule
}

func newHandlerTypeFormatterProvider(name string) *handlerTypeFormatterProvider {
	return &handlerTypeFormatterProvider{
		BaseModule: modules.NewBaseModule(name, modules.TypeHandler, 10),
	}
}

func (m *handlerTypeFormatterProvider) FormatterFunctions() []func([]string, stdslog.Attr) (stdslog.Value, bool) {
	return []func([]string, stdslog.Attr) (stdslog.Value, bool){
		func(_ []string, attr stdslog.Attr) (stdslog.Value, bool) {
			if attr.Key != "value" {
				return attr.Value, false
			}
			return stdslog.StringValue("incorrect:" + attr.Value.String()), true
		},
	}
}

func TestLoggerAppliesFormatterCallbacksOnlyFromFormatterModules(t *testing.T) {
	resetForTest()
	var buf bytes.Buffer
	logger := NewLogger(&buf, true, false)

	if err := logger.UseWithError(newHandlerTypeFormatterProvider("handler-formatter-provider")); err != nil {
		t.Fatalf("install handler module: %v", err)
	}
	logger.Info("message", "value", "original")

	if strings.Contains(buf.String(), "value=incorrect:original") {
		t.Fatalf("handler module formatter callback must not run: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "value=original") {
		t.Fatalf("expected original attribute value, got %q", buf.String())
	}
}

func TestLoggerUseWithErrorRejectsInvalidModules(t *testing.T) {
	logger := NewLogger(io.Discard, true, false)

	tests := []struct {
		name   string
		module modules.Module
	}{
		{name: "nil interface"},
		{name: "typed nil", module: (*trackingModule)(nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("UseWithError panicked for invalid module: %v", recovered)
				}
			}()
			if err := logger.UseWithError(tc.module); err == nil {
				t.Fatal("expected invalid module error")
			}
		})
	}
}

func TestLoggerUseStaysChainableForInvalidAndDisabledModules(t *testing.T) {
	logger := NewLogger(io.Discard, true, false)
	disabled := modules.NewBaseModule("disabled", modules.TypeFormatter, 10)
	disabled.SetEnabled(false)

	if got := logger.Use(nil); got != logger {
		t.Fatal("Use must return the receiving logger for an invalid module")
	}
	if err := logger.UseWithError(disabled); err != nil {
		t.Fatalf("disabled module must remain a no-op: %v", err)
	}
	if got := logger.Use(disabled); got != logger {
		t.Fatal("Use must return the receiving logger for a disabled module")
	}
	if got := logger.Diagnostics(); len(got) != 0 {
		t.Fatalf("disabled module must not be registered: %+v", got)
	}
}

func TestModuleBearingUnscopedLoggerInheritsGlobalRuntimeFormatter(t *testing.T) {
	resetForTest()
	var buf bytes.Buffer
	logger := NewLogger(&buf, true, false)
	if err := logger.UseWithError(newConfigurableTestFormatter("runtime-formatter-anchor", "other", "module:")); err != nil {
		t.Fatalf("install module: %v", err)
	}

	id := RegisterFormatter("post-module-runtime", func(_ []string, attr stdslog.Attr) (stdslog.Value, bool) {
		if attr.Key != "value" {
			return attr.Value, false
		}
		return stdslog.StringValue("runtime:" + attr.Value.String()), true
	})
	if id == "" {
		t.Fatal("register runtime formatter")
	}
	t.Cleanup(func() { RemoveFormatter(id) })

	logger.Info("message", "value", "original")
	if !strings.Contains(buf.String(), "value=runtime:original") {
		t.Fatalf("module-bearing unscoped logger must inherit runtime formatter: %q", buf.String())
	}
}

func TestModuleBearingUnscopedLoggerInheritsGlobalDLP(t *testing.T) {
	resetForTest()
	DisableDLPLogger()
	t.Cleanup(DisableDLPLogger)

	var buf bytes.Buffer
	logger := NewLogger(&buf, true, false)
	if err := logger.UseWithError(newConfigurableTestFormatter("dlp-anchor", "other", "module:")); err != nil {
		t.Fatalf("install module: %v", err)
	}

	EnableDLPLogger()
	logger.Info("login", "phone", "13812345678")
	if strings.Contains(buf.String(), "13812345678") {
		t.Fatalf("module-bearing unscoped logger must inherit global DLP: %q", buf.String())
	}
}

func TestIndependentLoggerLineagesAllowSameModuleName(t *testing.T) {
	resetForTest()
	EnableTextLogger()
	DisableJSONLogger()

	var firstBuf, secondBuf bytes.Buffer
	first := NewLogger(&firstBuf, true, false)
	second := NewLogger(&secondBuf, true, false)
	if err := first.UseWithError(newConfigurableTestFormatter("shared", "value", "first:")); err != nil {
		t.Fatalf("install first module: %v", err)
	}
	if err := second.UseWithError(newConfigurableTestFormatter("shared", "value", "second:")); err != nil {
		t.Fatalf("install second module: %v", err)
	}

	first.Info("first", "value", "x")
	second.Info("second", "value", "x")
	if !strings.Contains(firstBuf.String(), "value=first:x") {
		t.Fatalf("first lineage missing formatter: %q", firstBuf.String())
	}
	if !strings.Contains(secondBuf.String(), "value=second:x") {
		t.Fatalf("second lineage missing formatter: %q", secondBuf.String())
	}
}

func TestDerivedLoggersShareModuleCatalog(t *testing.T) {
	resetForTest()
	EnableTextLogger()
	DisableJSONLogger()

	var buf bytes.Buffer
	parent := NewLogger(&buf, true, false)
	lineage := []*Logger{
		parent,
		parent.With("scope", "child"),
		parent.WithGroup("group"),
		parent.WithContext(context.Background()),
	}
	if err := lineage[1].UseWithError(newConfigurableTestFormatter("lineage", "value", "shared:")); err != nil {
		t.Fatalf("install through child: %v", err)
	}
	for i, logger := range lineage {
		logger.Info("lineage", "value", i)
	}
	if got := strings.Count(buf.String(), "value=shared:"); got != len(lineage) {
		t.Fatalf("expected all lineage members to format, got %d in %q", got, buf.String())
	}
}

func TestIndependentConstructorsInitializeModuleCatalog(t *testing.T) {
	resetForTest()
	constructors := []struct {
		name string
		new  func() *Logger
	}{
		{name: "logger", new: func() *Logger { return NewLogger(io.Discard, true, false) }},
		{name: "config", new: func() *Logger { return NewLoggerWithConfig(io.Discard, DefaultConfig()) }},
		{name: "logfmt", new: func() *Logger { return NewLogfmtLogger(io.Discard, nil) }},
		{name: "gelf", new: func() *Logger { return NewGELFLogger(io.Discard, nil, nil) }},
		{name: "builder", new: func() *Logger { return NewLoggerBuilder().WithWriter(io.Discard).Build() }},
	}
	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			logger := tc.new()
			module := modules.NewHandlerModule("module-"+tc.name, stdslog.NewTextHandler(io.Discard, nil))
			if err := logger.UseWithError(module); err != nil {
				t.Fatalf("install module: %v", err)
			}
			if got := logger.Diagnostics(); len(got) != 1 || got[0].Name != "module-"+tc.name {
				t.Fatalf("unexpected diagnostics: %+v", got)
			}
		})
	}
}

func TestLoggerLineageRejectsDuplicateModuleName(t *testing.T) {
	logger := NewLogger(&bytes.Buffer{}, true, false)
	if err := logger.UseWithError(newConfigurableTestFormatter("duplicate", "value", "one:")); err != nil {
		t.Fatalf("install first module: %v", err)
	}
	if got := logger.Use(newConfigurableTestFormatter("duplicate", "value", "ignored:")); got != logger {
		t.Fatal("Use must preserve its chainable return value")
	}
	if got := logger.Diagnostics(); len(got) != 1 {
		t.Fatalf("Use must ignore the duplicate without changing the catalog: %+v", got)
	}
	if err := logger.UseWithError(newConfigurableTestFormatter("duplicate", "value", "two:")); err == nil {
		t.Fatal("expected duplicate module error")
	}
}

func TestModuleObservationUsesLoggerLineage(t *testing.T) {
	resetForTest()
	first := NewLogger(&bytes.Buffer{}, true, false)
	second := NewLogger(&bytes.Buffer{}, true, false)
	firstModule := newConfigurableTestFormatter("first-only", "value", "first:")
	firstSecondModule := newConfigurableTestFormatter("first-second", "value", "next:")
	secondModule := newConfigurableTestFormatter("second-only", "value", "second:")
	if err := first.UseWithError(firstModule); err != nil {
		t.Fatal(err)
	}
	if err := first.UseWithError(firstSecondModule); err != nil {
		t.Fatal(err)
	}
	if err := second.UseWithError(secondModule); err != nil {
		t.Fatal(err)
	}
	SetDefault(first)

	if got := first.Diagnostics(); len(got) != 2 || got[0].Name != "first-only" || got[1].Name != "first-second" {
		t.Fatalf("unexpected first diagnostics: %+v", got)
	}
	if got := second.Diagnostics(); len(got) != 1 || got[0].Name != "second-only" {
		t.Fatalf("unexpected second diagnostics: %+v", got)
	}
	if got := RegisteredModules(); len(got) != 2 || got[0] != "first-only" || got[1] != "first-second" {
		t.Fatalf("unexpected default modules: %v", got)
	}
	if got := CollectModuleDiagnostics(); len(got) != 2 || got[0].Name != "first-only" || got[1].Name != "first-second" {
		t.Fatalf("unexpected default diagnostics: %+v", got)
	}
	if _, exists := modules.GetModule("first-only"); exists {
		t.Fatal("Logger.Use must not write into the legacy global registry")
	}
	if _, exists := modules.GetModule("first-second"); exists {
		t.Fatal("Logger.Use must not write the second module into the legacy global registry")
	}
}

func TestLoggerManagerConfigurePreservesModuleLineage(t *testing.T) {
	resetForTest()
	var buf bytes.Buffer
	manager := &LoggerManager{
		instances: make(map[string]*Logger),
		config: &GlobalConfig{
			DefaultWriter: &buf,
			DefaultLevel:  LevelInfo,
			EnableText:    true,
			EnableJSON:    false,
		},
	}
	logger := manager.GetDefault()
	if err := logger.UseWithError(newConfigurableTestFormatter("manager-lineage", "value", "preserved:")); err != nil {
		t.Fatalf("install module: %v", err)
	}
	if err := manager.Configure(manager.config); err != nil {
		t.Fatalf("configure manager: %v", err)
	}

	logger.Info("reconfigured", "value", "x")
	if !strings.Contains(buf.String(), "value=preserved:x") {
		t.Fatalf("manager configuration lost module lineage: %q", buf.String())
	}
}

func TestLoggerUpdateModuleConfigStaysWithinLineage(t *testing.T) {
	resetForTest()
	EnableTextLogger()
	DisableJSONLogger()

	var firstBuf, secondBuf bytes.Buffer
	first := NewLogger(&firstBuf, true, false)
	child := first.With("scope", "child")
	second := NewLogger(&secondBuf, true, false)
	firstModule := newConfigurableTestFormatter("updatable", "value", "old:")
	secondModule := newConfigurableTestFormatter("updatable", "value", "second:")
	if err := first.UseWithError(firstModule); err != nil {
		t.Fatal(err)
	}
	if err := second.UseWithError(secondModule); err != nil {
		t.Fatal(err)
	}
	if err := child.UpdateModuleConfig("updatable", modules.Config{"key": "value", "prefix": "new:"}); err != nil {
		t.Fatalf("update through child: %v", err)
	}

	first.Info("first", "value", "x")
	second.Info("second", "value", "x")
	if !strings.Contains(firstBuf.String(), "value=new:x") {
		t.Fatalf("lineage update missing: %q", firstBuf.String())
	}
	if !strings.Contains(secondBuf.String(), "value=second:x") {
		t.Fatalf("independent lineage changed: %q", secondBuf.String())
	}
}

type trackingModule struct {
	*modules.BaseModule
	mu    sync.Mutex
	calls int
	err   error
}

func newTrackingModule(name string) *trackingModule {
	return &trackingModule{BaseModule: modules.NewBaseModule(name, modules.TypeHandler, 100)}
}

func (m *trackingModule) Configure(config modules.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return m.err
	}
	return m.BaseModule.Configure(config)
}

func (m *trackingModule) configureCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestPackageUpdateFallsBackOnlyWhenDefaultModuleIsMissing(t *testing.T) {
	resetForTest()
	name := "legacy-fallback-test"
	legacy := newTrackingModule(name)
	if err := modules.RegisterModule(legacy); err != nil {
		t.Fatalf("register legacy module: %v", err)
	}
	t.Cleanup(func() { _ = modules.GetRegistry().Remove(name) })

	if err := UpdateModuleConfig(name, modules.Config{"enabled": true}); err != nil {
		t.Fatalf("fallback update: %v", err)
	}
	if got := legacy.configureCalls(); got != 1 {
		t.Fatalf("expected one legacy update, got %d", got)
	}
}

func TestPackageUpdateDoesNotHideDefaultModuleError(t *testing.T) {
	resetForTest()
	name := "default-error-test"
	wantErr := errors.New("configuration rejected")
	local := newConfigurableTestFormatter(name, "value", "local:")
	local.err = wantErr
	defaultLogger := NewLogger(&bytes.Buffer{}, true, false)
	if err := defaultLogger.UseWithError(local); err != nil {
		t.Fatal(err)
	}
	SetDefault(defaultLogger)

	legacy := newTrackingModule(name)
	if err := modules.RegisterModule(legacy); err != nil {
		t.Fatalf("register legacy module: %v", err)
	}
	t.Cleanup(func() { _ = modules.GetRegistry().Remove(name) })

	err := UpdateModuleConfig(name, modules.Config{"key": "value", "prefix": "new:"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected local configuration error, got %v", err)
	}
	if got := legacy.configureCalls(); got != 0 {
		t.Fatalf("legacy fallback must not run, got %d calls", got)
	}
}

func TestLoggerModuleCatalogConcurrentUse(t *testing.T) {
	resetForTest()
	EnableTextLogger()
	DisableJSONLogger()

	logger := NewLogger(io.Discard, true, false)
	module := newConfigurableTestFormatter("concurrent", "value", "a:")
	if err := logger.UseWithError(module); err != nil {
		t.Fatal(err)
	}

	var ready sync.WaitGroup
	var wg sync.WaitGroup
	start := make(chan struct{})
	ready.Add(12)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			ready.Done()
			<-start
			for n := 0; n < 200; n++ {
				logger.Info("concurrent", "value", worker)
				_ = logger.Diagnostics()
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			ready.Done()
			<-start
			for n := 0; n < 100; n++ {
				config := modules.Config{"key": "value", "prefix": fmt.Sprintf("%d:", worker)}
				if err := logger.UpdateModuleConfig("concurrent", config); err != nil {
					t.Errorf("update module: %v", err)
					return
				}
			}
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()
}
