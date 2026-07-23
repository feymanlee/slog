package slog

import (
	"errors"
	"fmt"
	stdslog "log/slog"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/darkit/slog/modules"
)

var (
	errLoggerModuleDuplicate = errors.New("slog: logger module already registered")
	errLoggerModuleInvalid   = errors.New("slog: invalid logger module")
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
	if module.Type() != modules.TypeFormatter {
		return nil
	}
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
	if c == nil || isInvalidLoggerModule(module) {
		return errLoggerModuleInvalid
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

func isInvalidLoggerModule(module modules.Module) bool {
	if module == nil {
		return true
	}

	value := reflect.ValueOf(module)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

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
