package modules

import (
	"fmt"
	"log/slog"
	"sync"
)

// ModuleType 定义模块类型
type ModuleType int

const (
	TypeFormatter ModuleType = iota // 格式化器
	TypeHandler                     // 处理器
	TypeSink                        // 日志接收器
)

func (mt ModuleType) String() string {
	switch mt {
	case TypeFormatter:
		return "formatter"
	case TypeHandler:
		return "handler"
	case TypeSink:
		return "sink"
	default:
		return "unknown"
	}
}

// Config 通用配置接口
type Config map[string]any

// ModuleConfig 模块配置
type ModuleConfig struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
	Config   Config `json:"config"`
}

// Module 定义模块接口。
type Module interface {
	// Name 返回模块名称
	Name() string
	// Type 返回模块类型
	Type() ModuleType
	// Configure 配置模块
	Configure(config Config) error
	// Handler 返回slog处理器
	Handler() slog.Handler
	// Priority 返回优先级，数字越小优先级越高
	Priority() int
	// Enabled 返回模块是否启用
	Enabled() bool
}

// ModuleFactory 模块工厂函数
type ModuleFactory func(config Config) (Module, error)

// Registry 模块注册中心
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ModuleFactory
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ModuleFactory),
	}
}

// RegisterFactory 注册模块工厂
func (r *Registry) RegisterFactory(name string, factory ModuleFactory) error {
	if factory == nil {
		return fmt.Errorf("factory %s cannot be nil", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("factory %s already registered", name)
	}

	r.factories[name] = factory
	return nil
}

// Create 通过工厂创建模块
func (r *Registry) Create(name string, config Config) (Module, error) {
	r.mu.RLock()
	factory, exists := r.factories[name]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("factory %s not found", name)
	}

	return factory(config)
}

// ListFactories 列出所有已注册的工厂名称
func (r *Registry) ListFactories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factories := make([]string, 0, len(r.factories))
	for name := range r.factories {
		factories = append(factories, name)
	}
	return factories
}

// BaseModule 基础模块实现
type BaseModule struct {
	name     string
	typ      ModuleType
	priority int
	enabled  bool
	handler  slog.Handler
	config   Config
}

// NewBaseModule 创建基础模块
func NewBaseModule(name string, typ ModuleType, priority int) *BaseModule {
	return &BaseModule{
		name:     name,
		typ:      typ,
		priority: priority,
		enabled:  true,
		config:   make(Config),
	}
}

func (m *BaseModule) Name() string          { return m.name }
func (m *BaseModule) Type() ModuleType      { return m.typ }
func (m *BaseModule) Priority() int         { return m.priority }
func (m *BaseModule) Enabled() bool         { return m.enabled }
func (m *BaseModule) Handler() slog.Handler { return m.handler }

func (m *BaseModule) Configure(config Config) error {
	m.config = config
	return nil
}

func (m *BaseModule) SetHandler(handler slog.Handler) {
	m.handler = handler
}

func (m *BaseModule) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// 全局注册中心
var globalRegistry = NewRegistry()

// RegisterFactory 全局注册工厂
func RegisterFactory(name string, factory ModuleFactory) error {
	return globalRegistry.RegisterFactory(name, factory)
}

// NewHandlerModule 便捷创建处理器模块，默认优先级 100。
func NewHandlerModule(name string, handler slog.Handler) Module {
	m := NewBaseModule(name, TypeHandler, 100)
	m.SetHandler(handler)
	return m
}

// CreateModule 全局创建模块
func CreateModule(name string, config Config) (Module, error) {
	return globalRegistry.Create(name, config)
}

// GetRegistry 获取全局注册中心
func GetRegistry() *Registry {
	return globalRegistry
}
