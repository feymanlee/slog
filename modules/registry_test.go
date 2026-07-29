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

func TestRegistryModuleLifecycleAndPriorityOrder(t *testing.T) {
	registry := NewRegistry()
	slow := &registryTestModule{name: "slow", typ: TypeHandler, priority: 200}
	fast := &registryTestModule{name: "fast", typ: TypeHandler, priority: 10}
	sink := &registryTestModule{name: "sink", typ: TypeSink, priority: 1}

	for _, module := range []Module{slow, fast, sink} {
		if err := registry.Register(module); err != nil {
			t.Fatalf("Register(%q) error = %v", module.Name(), err)
		}
	}
	if err := registry.Register(fast); err == nil {
		t.Fatal("duplicate Register() error = nil")
	}

	handlers := registry.GetByType(TypeHandler)
	if len(handlers) != 2 || handlers[0] != fast || handlers[1] != slow {
		t.Fatalf("handler order = %v, want fast then slow", moduleNames(handlers))
	}
	handlers[0] = nil
	if got := registry.GetByType(TypeHandler)[0]; got != fast {
		t.Fatal("GetByType() returned registry-owned slice")
	}

	config := Config{"endpoint": "localhost"}
	if err := registry.Update("fast", config); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	fast.mu.Lock()
	gotEndpoint := fast.config["endpoint"]
	fast.mu.Unlock()
	if gotEndpoint != "localhost" {
		t.Fatalf("configured endpoint = %v", gotEndpoint)
	}

	if err := registry.Remove("fast"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, ok := registry.Get("fast"); ok {
		t.Fatal("removed module is still returned by Get()")
	}
	if got := registry.GetByType(TypeHandler); len(got) != 1 || got[0] != slow {
		t.Fatalf("handler chain after Remove() = %v", moduleNames(got))
	}
	if err := registry.Remove("missing"); err == nil {
		t.Fatal("Remove(missing) error = nil")
	}
	if err := registry.Update("missing", nil); err == nil {
		t.Fatal("Update(missing) error = nil")
	}
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

func TestRegistryRejectsNilRegistrations(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(nil); err == nil {
		t.Fatal("Register(nil) error = nil")
	}
	var typedNil *registryTestModule
	if err := registry.Register(typedNil); err == nil {
		t.Fatal("Register(typed nil) error = nil")
	}
	if err := registry.RegisterFactory("nil", nil); err == nil {
		t.Fatal("RegisterFactory(nil) error = nil")
	}
}

func TestRegistrySupportsConcurrentIndependentRegistrations(t *testing.T) {
	registry := NewRegistry()
	const count = 64
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			name := fmt.Sprintf("module-%02d", index)
			module := &registryTestModule{name: name, typ: TypeHandler, priority: index}
			if err := registry.Register(module); err != nil {
				t.Errorf("Register(%q) error = %v", name, err)
				return
			}
			if got, ok := registry.Get(name); !ok || got != module {
				t.Errorf("Get(%q) = %v, %v", name, got, ok)
			}
		}(i)
	}
	wg.Wait()

	if got := len(registry.List()); got != count {
		t.Fatalf("List() length = %d, want %d", got, count)
	}
	ordered := registry.GetByType(TypeHandler)
	for i, module := range ordered {
		if module.Priority() != i {
			t.Fatalf("priority at index %d = %d", i, module.Priority())
		}
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

func moduleNames(modules []Module) []string {
	names := make([]string, len(modules))
	for i, module := range modules {
		if module != nil {
			names[i] = module.Name()
		}
	}
	return names
}
