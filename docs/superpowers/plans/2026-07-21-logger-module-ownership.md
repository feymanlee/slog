# Logger Module Ownership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give each Logger Lineage sole ownership of its Logger Module instances while preserving the legacy global registry interface.

**Architecture:** Add a private `loggerLineage` containing one synchronized `moduleCatalog`; independently constructed Loggers get distinct lineages while derived Loggers share one. Route formatter module execution, configuration, names, and diagnostics through the catalog, leaving DLP/runtime formatters in `extensions` and leaving handler/sink delivery deferred.

**Tech Stack:** Go 1.23, standard `log/slog`, standard `sync`, existing `modules.Module` and `modules.FormatterProvider` interfaces.

## Global Constraints

- Preserve every existing exported function and type signature.
- Different Logger Lineages may install Logger Modules with the same name; one lineage rejects duplicates.
- `With`, `WithGroup`, and `WithContext` share the parent Logger Lineage.
- `TypeHandler` and `TypeSink` delivery remains unchanged and deferred.
- Package-level observation and updates target the Default Logger.
- Fall back to the legacy global registry only when the Default Logger does not own the named module.
- Keep existing global DLP inheritance and runtime formatter registration behavior unchanged.
- Use TDD for every behavior change and do not skip git hooks.

---

## File Structure

- Create `logger_modules.go`: private Logger Lineage, module catalog, catalog errors, formatter snapshot, and local update behavior.
- Create `logger_modules_test.go`: Logger-interface regression tests plus small test Logger Module adapters.
- Modify `logger.go`: store the lineage on `Logger`, share it from `clone`, and pass it to handlers.
- Modify `log.go`: initialize a fresh lineage in every independent constructor and pass it to handlers.
- Modify `builder.go`: initialize a fresh lineage for the direct network-output construction path.
- Modify `logger_manager.go`: initialize/preserve lineages for default and named Loggers and rebuild handlers with the same lineage.
- Modify `context.go`: preserve the lineage when cloning handlers with context.
- Modify `logger_extend.go`: remove module instance bookkeeping from `extensions`; let `eHandler` apply the lineage formatter snapshot.
- Modify `logger_integration.go`: install and update through the lineage catalog; make package-level names and updates target the Default Logger.
- Modify `logger_diagnostics.go`: derive instance and package-level diagnostics from the lineage catalog.
- Modify `modules/formatter/adapter.go`: make formatter replacement atomic and race-free.
- Create `modules/formatter/adapter_test.go`: verify replacement and failed-update behavior.
- Modify `modules/README.md`: document Logger Lineage ownership and the deferred handler/sink delivery scope.

---

### Task 1: Make Built-In Formatter Configuration Replace Atomically

**Files:**
- Create: `modules/formatter/adapter_test.go`
- Modify: `modules/formatter/adapter.go:3-68`

**Interfaces:**
- Consumes: `modules.Config.Bind(target any) error`, `Formatter`, `ErrorFormatter(fieldName string) Formatter`.
- Produces: unchanged `(*FormatterAdapter).Configure(modules.Config) error` and `(*FormatterAdapter).FormatterFunctions() []func([]string, slog.Attr) (slog.Value, bool)` with replacement semantics.

- [ ] **Step 1: Write failing replacement and rollback tests**

```go
package formatter

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/darkit/slog/modules"
)

func TestFormatterAdapterConfigureReplacesPreviousFormatters(t *testing.T) {
	adapter := NewFormatterAdapter()
	if err := adapter.Configure(modules.Config{"type": "error", "replacement": "error"}); err != nil {
		t.Fatalf("configure initial formatter: %v", err)
	}
	if err := adapter.Configure(modules.Config{"type": "error", "replacement": "err"}); err != nil {
		t.Fatalf("replace formatter: %v", err)
	}

	funcs := adapter.FormatterFunctions()
	if len(funcs) != 1 {
		t.Fatalf("expected one replacement formatter, got %d", len(funcs))
	}
	if _, ok := funcs[0](nil, slog.Any("error", errors.New("old"))); ok {
		t.Fatal("expected previous error formatter to be removed")
	}
	if _, ok := funcs[0](nil, slog.Any("err", errors.New("current"))); !ok {
		t.Fatal("expected replacement formatter to handle err")
	}
}

func TestFormatterAdapterConfigureFailureKeepsPreviousFormatters(t *testing.T) {
	adapter := NewFormatterAdapter()
	if err := adapter.Configure(modules.Config{"type": "error", "replacement": "error"}); err != nil {
		t.Fatalf("configure initial formatter: %v", err)
	}
	if err := adapter.Configure(modules.Config{"type": make(chan int)}); err == nil {
		t.Fatal("expected invalid config to fail")
	}

	funcs := adapter.FormatterFunctions()
	if len(funcs) != 1 {
		t.Fatalf("expected initial formatter to survive, got %d", len(funcs))
	}
	if _, ok := funcs[0](nil, slog.Any("error", errors.New("still active"))); !ok {
		t.Fatal("expected initial formatter to remain active")
	}
}
```

- [ ] **Step 2: Run the focused tests and verify the first test fails**

Run: `go test ./modules/formatter -run 'TestFormatterAdapterConfigure' -count=1`

Expected: FAIL with `expected one replacement formatter, got 2`.

- [ ] **Step 3: Replace the formatter slice only after successful parsing**

Update `FormatterAdapter` and its methods to use the following synchronization and replacement flow:

```go
type FormatterAdapter struct {
	*modules.BaseModule
	mu         sync.RWMutex
	formatters []Formatter
}

func (f *FormatterAdapter) Configure(config modules.Config) error {
	var cfg struct {
		Type        string `json:"type"`
		Format      string `json:"format"`
		Replacement string `json:"replacement"`
	}
	if err := config.Bind(&cfg); err != nil {
		return err
	}

	next := make([]Formatter, 0, 1)
	switch cfg.Type {
	case "time":
		format := cfg.Format
		if format == "" {
			format = "2006-01-02 15:04:05"
		}
		next = append(next, TimeFormatter(format, time.Local))
	case "error":
		replacement := cfg.Replacement
		if replacement == "" {
			replacement = "error"
		}
		next = append(next, ErrorFormatter(replacement))
	}

	if err := f.BaseModule.Configure(config); err != nil {
		return err
	}
	f.mu.Lock()
	f.formatters = next
	f.mu.Unlock()
	return nil
}

func (f *FormatterAdapter) FormatterFunctions() []func([]string, slog.Attr) (slog.Value, bool) {
	f.mu.RLock()
	formatters := append([]Formatter(nil), f.formatters...)
	f.mu.RUnlock()

	funcs := make([]func([]string, slog.Attr) (slog.Value, bool), 0, len(formatters))
	for _, formatter := range formatters {
		lf := formatter
		funcs = append(funcs, func(groups []string, attr slog.Attr) (slog.Value, bool) {
			return lf(groups, attr)
		})
	}
	return funcs
}
```

Add `sync` to the import block. Do not change constructor or factory registration signatures.

- [ ] **Step 4: Run formatter tests**

Run: `go test ./modules/formatter -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the formatter behavior**

```bash
git add modules/formatter/adapter.go modules/formatter/adapter_test.go
git commit -m "fix(formatter): replace module configuration atomically"
```

---

### Task 2: Move Logger Module Ownership Into Logger Lineages

**Files:**
- Create: `logger_modules.go`
- Create: `logger_modules_test.go`
- Modify: `logger.go:305-319,350-425,755-779,913-950`
- Modify: `log.go:349-449,734-758`
- Modify: `builder.go:115-170`
- Modify: `logger_manager.go:55-128,214-274`
- Modify: `context.go:61-78`
- Modify: `logger_extend.go:63-255,313-387,439-479`
- Modify: `logger_integration.go:23-78`
- Modify: `logger_diagnostics.go:15-48`

**Interfaces:**
- Consumes: `modules.Module`, `modules.FormatterProvider`, `FormatterFunc`, and existing Logger constructors.
- Produces: private `newLoggerLineage() *loggerLineage`, `(*moduleCatalog).register(modules.Module) error`, `(*moduleCatalog).names() []string`, `(*moduleCatalog).snapshot() []modules.Module`, and unchanged exported Logger installation/diagnostic interfaces.

- [ ] **Step 1: Write Logger-interface ownership tests and test adapter**

Create `logger_modules_test.go` with this adapter and tests:

```go
package slog

import (
	"bytes"
	"context"
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

```

- [ ] **Step 2: Run the ownership tests and verify they fail**

Run: `go test . -run 'Test(IndependentLoggerLineages|IndependentConstructors|DerivedLoggers|LoggerLineageRejects|ModuleObservation)' -count=1`

Expected: FAIL because the second same-named module hits the global registry, derived handlers do not share a lineage formatter snapshot, and observations read global `extensions`.

- [ ] **Step 3: Add the private lineage and catalog**

Create `logger_modules.go` with these private types and operations:

```go
package slog

import (
	"errors"
	"fmt"
	stdslog "log/slog"
	"sync"
	"sync/atomic"

	"github.com/darkit/slog/modules"
)

var (
	errLoggerModuleDuplicate = errors.New("slog: logger module already registered")
	errLoggerModuleNotFound  = errors.New("slog: logger module not found")
)

type loggerLineage struct {
	modules *moduleCatalog
}

func newLoggerLineage() *loggerLineage {
	return &loggerLineage{modules: newModuleCatalog()}
}

type moduleCatalogEntry struct {
	module      modules.Module
	configureMu sync.Mutex
	formatters  []FormatterFunc
}

type moduleCatalog struct {
	mu                sync.RWMutex
	entries           map[string]*moduleCatalogEntry
	order             []string
	formatterSnapshot atomic.Value // []FormatterFunc
}

func newModuleCatalog() *moduleCatalog {
	catalog := &moduleCatalog{entries: make(map[string]*moduleCatalogEntry)}
	catalog.formatterSnapshot.Store([]FormatterFunc(nil))
	return catalog
}

func formatterFunctions(module modules.Module) []FormatterFunc {
	provider, ok := module.(modules.FormatterProvider)
	if !ok {
		return nil
	}
	provided := provider.FormatterFunctions()
	formatters := make([]FormatterFunc, 0, len(provided))
	for _, fn := range provided {
		if fn != nil {
			formatters = append(formatters, FormatterFunc(fn))
		}
	}
	return formatters
}

func (c *moduleCatalog) register(module modules.Module) error {
	if c == nil || module == nil {
		return errors.New("slog: invalid logger module")
	}
	name := module.Name()
	entry := &moduleCatalogEntry{module: module, formatters: formatterFunctions(module)}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[name]; exists {
		return fmt.Errorf("%w: %s", errLoggerModuleDuplicate, name)
	}
	c.entries[name] = entry
	c.order = append(c.order, name)
	c.rebuildFormatterSnapshotLocked()
	return nil
}

func (c *moduleCatalog) rebuildFormatterSnapshotLocked() {
	formatters := make([]FormatterFunc, 0)
	for _, name := range c.order {
		formatters = append(formatters, c.entries[name].formatters...)
	}
	c.formatterSnapshot.Store(formatters)
}

func (c *moduleCatalog) names() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.order...)
}

func (c *moduleCatalog) snapshot() []modules.Module {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]modules.Module, 0, len(c.order))
	for _, name := range c.order {
		result = append(result, c.entries[name].module)
	}
	return result
}

func (c *moduleCatalog) applyFormatters(groups []string, attr stdslog.Attr) stdslog.Attr {
	if c == nil {
		return attr
	}
	formatters, _ := c.formatterSnapshot.Load().([]FormatterFunc)
	for _, formatter := range formatters {
		if value, ok := formatter(groups, attr); ok {
			attr.Value = value
		}
	}
	return attr
}

func (c *moduleCatalog) hasFormatters() bool {
	if c == nil {
		return false
	}
	formatters, _ := c.formatterSnapshot.Load().([]FormatterFunc)
	return len(formatters) > 0
}
```

- [ ] **Step 4: Wire one lineage through every Logger and eHandler path**

Add `lineage *loggerLineage` to both `Logger` and `eHandler`. Change handler helpers to these exact signatures and copy the lineage in every returned handler:

```go
func newAddonsHandler(next slog.Handler, opts *extensions, lineage *loggerLineage) *eHandler
func rebindAddonsHandler(handler slog.Handler, opts *extensions, lineage *loggerLineage) slog.Handler
func cloneHandlerWithContext(handler slog.Handler, ctx context.Context, opts *extensions, lineage *loggerLineage) slog.Handler
```

Use `lineage: lineage` in the fresh, rebind, and context-clone handler literals.
Use `lineage: h.lineage` in both derived handler literals. For `WithAttrs`, the
complete literal is:

```go
return &eHandler{
	handler:     h.handler,
	opts:        h.opts,
	lineage:     h.lineage,
	groups:      slices.Clone(h.groups),
	prefixes:    slices.Clone(h.prefixes),
	observerOps: append(cloneObserverOperations(h.observerOps), observerOperation{kind: observerOpAttrs, attrs: slices.Clone(attrs)}),
	ctx:         h.ctx,
}
```

Add the same `lineage: h.lineage` field to the existing `WithGroup` literal.

For each independent construction path, add these exact statements: create the
lineage before its `Logger` literal, add the field to that literal, and pass the
same pointer when creating each handler.

```go
lineage := newLoggerLineage()
```

```go
lineage: lineage,
```

For the text and JSON constructor calls, the resulting expressions are:

```go
logger.text = slog.New(newAddonsHandler(NewConsoleHandler(logger.w, logger.noColor, options), logger.ext, lineage))
logger.json = slog.New(newAddonsHandler(NewJSONHandler(logger.w, options), logger.ext, lineage))
```

Apply that pattern in `NewLogger`, `NewLogfmtLogger`, `NewGELFLogger`,
`NewLoggerWithConfig`, `LoggerBuilder.Build`'s `output.net` branch,
`LoggerManager.createLoggerWithConfig`, and `LoggerManager.setDefaultSlogLogger`.
`LoggerManager.setDefaultLogger` must preserve the supplied Logger's lineage and
pass it to `rebuildLoggerHandlers`.

When adapting a standard Logger in `setDefaultSlogLogger`, wrap its existing
handler so package-level enhanced logging can execute lineage formatters:

```go
lineage := newLoggerLineage()
wrapped := slog.New(newAddonsHandler(logger.Handler(), ext, lineage))
lm.defaultLogger = &Logger{
	text:         wrapped,
	ctx:          context.Background(),
	level:        levelVar.Level(),
	levelVar:     newLoggerLevel(levelVar.Level(), false),
	ext:          ext,
	lineage:      lineage,
	config:       cfg,
	renderConfig: outputRenderConfig{},
}
```

In `Default(modules ...string)`, pass `newLogger.lineage` when rebuilding both
the cloned text and JSON handlers. In `Logger.scopeExtensions`, pass `l.lineage`
to both `rebindAddonsHandler` calls.

In `Logger.clone`, copy the existing pointer:

```go
lineage: l.lineage,
```

In `context.go`, pass `newLogger.lineage` to both `cloneHandlerWithContext` calls.

- [ ] **Step 5: Route installation, formatter execution, names, and diagnostics through the catalog**

Replace `Logger.UseWithError` with:

```go
func (l *Logger) UseWithError(module modules.Module) error {
	if l == nil {
		return errors.New("slog logger is nil")
	}
	if module == nil || !module.Enabled() {
		return nil
	}
	if l.lineage == nil || l.lineage.modules == nil {
		return errors.New("slog logger lineage is not initialized")
	}
	return l.lineage.modules.register(module)
}
```

Do not call `scopeExtensions` from module installation. In
`eHandler.transformAttr`, replace the current formatter and DLP block with this
nil-safe order: runtime formatters, lineage formatters, then DLP.

```go
if h.opts != nil {
	before := attr
	attr = h.opts.applyFormatters(groups, attr)
	h.opts.emitDiagnostics("formatter", groups, before, attr)
}
if h.lineage != nil {
	before := attr
	attr = h.lineage.modules.applyFormatters(groups, attr)
	if h.opts != nil {
		h.opts.emitDiagnostics("module_formatter", groups, before, attr)
	}
}
if h.opts != nil && h.opts.dlpEnabled.Load() && h.opts.dlpEngine != nil {
	before := attr
	switch attr.Value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(desensitizeAttrValue(h.opts.dlpEngine, attr.Key, attr.Value.String()))
	case slog.KindGroup:
		attrs := attr.Value.Group()
		newAttrs := make([]slog.Attr, len(attrs))
		for i, child := range attrs {
			newAttrs[i] = h.transformAttr(append(groups, attr.Key), child)
		}
		attr.Value = slog.GroupValue(newAttrs...)
	}
	h.opts.emitDiagnostics("dlp", groups, before, attr)
}
```

Update `eHandler.canPassThrough` so a lineage formatter prevents bypassing
attribute transformation:

```go
if h.lineage != nil && h.lineage.modules.hasFormatters() {
	return false
}
```

Place this check after the existing prefix check and before the existing
message/attribute transformer checks.

Remove `moduleRegistry`, `registeredModules`, `modulesMu`, and `moduleIndex` from
`extensions`, and remove `registerModule`, `addFormatterFuncs`,
`addFormattersFromModule`, and `snapshotModules` from `logger_extend.go`.
Remove the `modules` import from `logger_extend.go` after those functions are
deleted.

Make package-level names use the Default Logger catalog:

```go
func RegisteredModules() []string {
	logger := GetGlobalLogger()
	if logger == nil || logger.lineage == nil {
		return nil
	}
	return logger.lineage.modules.names()
}
```

Change diagnostics to accept a module slice and resolve the correct catalog:

```go
func (l *Logger) Diagnostics() []ModuleDiagnostics {
	if l == nil || l.lineage == nil {
		return nil
	}
	return collectModuleDiagnostics(l.lineage.modules.snapshot())
}

func CollectModuleDiagnostics() []ModuleDiagnostics {
	return GetGlobalLogger().Diagnostics()
}

func collectModuleDiagnostics(moduleList []modules.Module) []ModuleDiagnostics {
	diags := make([]ModuleDiagnostics, 0, len(moduleList))
	for _, m := range moduleList {
		diag := ModuleDiagnostics{
			Name:     m.Name(),
			Type:     m.Type(),
			Enabled:  m.Enabled(),
			Priority: m.Priority(),
		}
		if healthable, ok := m.(interface {
			HealthCheck() error
			IsHealthy() bool
		}); ok {
			err := healthable.HealthCheck()
			healthy := err == nil && healthable.IsHealthy()
			diag.Healthy = &healthy
		}
		if measurable, ok := m.(interface{ GetMetrics() map[string]any }); ok {
			diag.Metrics = measurable.GetMetrics()
		}
		diags = append(diags, diag)
	}
	return diags
}
```

- [ ] **Step 6: Run ownership tests and the existing root tests**

Run: `go test . -run 'Test(IndependentLoggerLineages|DerivedLoggers|LoggerLineageRejects|ModuleObservation|LoggerUseFormatter)' -count=1`

Expected: PASS.

Run: `go test . -count=1`

Expected: PASS.

- [ ] **Step 7: Commit lineage ownership**

```bash
git add logger_modules.go logger_modules_test.go logger.go log.go builder.go logger_manager.go context.go logger_extend.go logger_integration.go logger_diagnostics.go
git commit -m "fix(logger): scope module ownership to logger lineages"
```

---

### Task 3: Add Local Updates and Default-Logger Compatibility Fallback

**Files:**
- Modify: `logger_modules.go`
- Modify: `logger_modules_test.go`
- Modify: `logger_integration.go:62-65`

**Interfaces:**
- Consumes: Task 2's `loggerLineage`, `moduleCatalog`, `moduleCatalogEntry`, and `errLoggerModuleNotFound`.
- Produces: `(*Logger).UpdateModuleConfig(name string, config modules.Config) error` and the existing package-level `UpdateModuleConfig(name string, config modules.Config) error` with Default Logger precedence.

- [ ] **Step 1: Add failing local-update and fallback tests**

Append these tests and helper to `logger_modules_test.go`:

```go
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
```

Add the standard `errors` import to `logger_modules_test.go`.

- [ ] **Step 2: Run update tests and verify compilation fails**

Run: `go test . -run 'Test(LoggerUpdateModuleConfig|PackageUpdate)' -count=1`

Expected: FAIL to compile because `(*Logger).UpdateModuleConfig` does not exist.

- [ ] **Step 3: Implement serialized catalog updates**

Add this method to `logger_modules.go`:

```go
func (c *moduleCatalog) update(name string, config modules.Config) error {
	if c == nil {
		return fmt.Errorf("%w: %s", errLoggerModuleNotFound, name)
	}
	c.mu.RLock()
	entry, exists := c.entries[name]
	c.mu.RUnlock()
	if !exists {
		return fmt.Errorf("%w: %s", errLoggerModuleNotFound, name)
	}

	entry.configureMu.Lock()
	defer entry.configureMu.Unlock()
	if err := entry.module.Configure(config); err != nil {
		return err
	}
	formatters := formatterFunctions(entry.module)
	c.mu.Lock()
	entry.formatters = formatters
	c.rebuildFormatterSnapshotLocked()
	c.mu.Unlock()
	return nil
}
```

The catalog lock is not held while invoking third-party `Configure` or
`FormatterFunctions` code. The per-entry lock serializes updates to one module.

- [ ] **Step 4: Implement instance update and package-level fallback**

In `logger_integration.go`, add the instance method and replace the existing
package-level pass-through:

```go
func (l *Logger) UpdateModuleConfig(name string, config modules.Config) error {
	if l == nil || l.lineage == nil || l.lineage.modules == nil {
		return fmt.Errorf("%w: %s", errLoggerModuleNotFound, name)
	}
	return l.lineage.modules.update(name, config)
}

func UpdateModuleConfig(name string, config modules.Config) error {
	logger := GetGlobalLogger()
	if err := logger.UpdateModuleConfig(name, config); err != nil {
		if errors.Is(err, errLoggerModuleNotFound) {
			return modules.UpdateModuleConfig(name, config)
		}
		return err
	}
	return nil
}
```

Add `fmt` to `logger_integration.go` imports and retain its existing `errors`
import for `errors.Is` and routing behavior.

- [ ] **Step 5: Run update and root-package tests**

Run: `go test . -run 'Test(LoggerUpdateModuleConfig|PackageUpdate)' -count=1`

Expected: PASS.

Run: `go test . -count=1`

Expected: PASS.

- [ ] **Step 6: Commit update behavior**

```bash
git add logger_modules.go logger_modules_test.go logger_integration.go
git commit -m "feat(logger): add lineage-local module updates"
```

---

### Task 4: Verify Concurrency, Document Ownership, and Run Full Gates

**Files:**
- Modify: `logger_modules_test.go`
- Modify: `modules/README.md:13-25`

**Interfaces:**
- Consumes: completed Logger Lineage catalog, instance update, package-level fallback, and formatter replacement behavior.
- Produces: documented ownership semantics and race-verified behavior; no new exported interface.

- [ ] **Step 1: Add a concurrent logging, update, and diagnostics test**

Append this test to `logger_modules_test.go`:

```go
func TestLoggerModuleCatalogConcurrentUse(t *testing.T) {
	resetForTest()
	EnableTextLogger()
	DisableJSONLogger()

	logger := NewLogger(io.Discard, true, false)
	module := newConfigurableTestFormatter("concurrent", "value", "a:")
	if err := logger.UseWithError(module); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
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
			for n := 0; n < 100; n++ {
				config := modules.Config{"key": "value", "prefix": fmt.Sprintf("%d:", worker)}
				if err := logger.UpdateModuleConfig("concurrent", config); err != nil {
					t.Errorf("update module: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
```

Add `fmt` to the test imports; `io` was added by Task 2.

- [ ] **Step 2: Run the focused race test**

Run: `go test -race . -run TestLoggerModuleCatalogConcurrentUse -count=1`

Expected: PASS with no race report.

- [ ] **Step 3: Document ownership and current delivery scope**

Add this section after `modules/README.md`'s registration example:

```markdown
## Logger ownership

`Logger.Use` installs a module into that Logger Lineage. Loggers derived with
`With`, `WithGroup`, or `WithContext` share the same module catalog; separately
constructed Loggers own independent catalogs and may use the same module name.

Use `logger.UpdateModuleConfig(name, config)` for lineage-local updates. The
package-level `slog.UpdateModuleConfig` targets the Default Logger and falls back
to the legacy global registry only when the Default Logger does not own the
named module.

Formatter modules participate in the Logger formatting path. Handler and sink
module delivery remains separate until the async output lifecycle is unified.
```

- [ ] **Step 4: Run formatting and full test suite**

Run: `gofmt -w logger_modules.go logger_modules_test.go logger.go log.go builder.go logger_manager.go context.go logger_extend.go logger_integration.go logger_diagnostics.go modules/formatter/adapter.go modules/formatter/adapter_test.go`

Expected: command exits 0.

Run: `go test ./... -count=1`

Expected: PASS for every package.

- [ ] **Step 5: Run the full race suite**

Run: `go test -race ./... -count=1`

Expected: PASS with no race report.

- [ ] **Step 6: Review the final diff against the design**

Run: `git diff --check`

Expected: no output and exit 0.

Run: `git status --short`

Expected: only `logger_modules_test.go` and `modules/README.md` remain modified
after Tasks 1-3 have been committed.

- [ ] **Step 7: Commit tests and documentation**

```bash
git add logger_modules_test.go modules/README.md
git commit -m "test(logger): verify module lineage concurrency"
```
