package modules

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"
)

type registryTestModule struct {
	name     string
	typ      ModuleType
	priority int
	mu       sync.Mutex
	config   Config
}

func (m *registryTestModule) Name() string          { return m.name }
func (m *registryTestModule) Type() ModuleType      { return m.typ }
func (m *registryTestModule) Priority() int         { return m.priority }
func (m *registryTestModule) Enabled() bool         { return true }
func (m *registryTestModule) Handler() slog.Handler { return nil }
func (m *registryTestModule) Configure(config Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return nil
}

func TestRegistryFactoryCreatesConfiguredModule(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterFactory("handler", func(config Config) (Module, error) {
		priority, ok := config["priority"].(int)
		if !ok {
			return nil, fmt.Errorf("priority is required")
		}
		return &registryTestModule{name: "created", typ: TypeHandler, priority: priority}, nil
	}); err != nil {
		t.Fatalf("RegisterFactory() error = %v", err)
	}
	if err := registry.RegisterFactory("handler", func(Config) (Module, error) { return nil, nil }); err == nil {
		t.Fatal("duplicate RegisterFactory() error = nil")
	}

	created, err := registry.Create("handler", Config{"priority": 25})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Name() != "created" || created.Priority() != 25 {
		t.Fatalf("created module = %q priority %d", created.Name(), created.Priority())
	}
	if _, err := registry.Create("missing", nil); err == nil {
		t.Fatal("Create(missing) error = nil")
	}

	factories := registry.ListFactories()
	if len(factories) != 1 || factories[0] != "handler" {
		t.Fatalf("ListFactories() = %v", factories)
	}
}

func TestBaseModuleExposesConfiguredState(t *testing.T) {
	handler := slog.NewTextHandler(discardWriter{}, nil)
	module := NewBaseModule("audit", TypeHandler, 42)
	module.SetHandler(handler)
	module.SetEnabled(false)
	if err := module.Configure(Config{"format": "json"}); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if module.Name() != "audit" || module.Type() != TypeHandler || module.Priority() != 42 {
		t.Fatalf("module identity = %q, %v, %d", module.Name(), module.Type(), module.Priority())
	}
	if module.Enabled() || module.Handler() != handler || module.config["format"] != "json" {
		t.Fatal("base module state was not retained")
	}
	if TypeFormatter.String() != "formatter" || TypeHandler.String() != "handler" || TypeSink.String() != "sink" || ModuleType(99).String() != "unknown" {
		t.Fatal("ModuleType.String() returned an unexpected value")
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
