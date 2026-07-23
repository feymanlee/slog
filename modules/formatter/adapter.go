package formatter

import (
	"log/slog"
	"sync"
	"time"

	"github.com/darkit/slog/modules"
)

// FormatterAdapter 格式化器模块适配器
type FormatterAdapter struct {
	*modules.BaseModule
	mu         sync.RWMutex
	formatters []Formatter
}

// NewFormatterAdapter 创建格式化器适配器
func NewFormatterAdapter() *FormatterAdapter {
	return &FormatterAdapter{
		BaseModule: modules.NewBaseModule("formatter", modules.TypeFormatter, 10),
		formatters: make([]Formatter, 0),
	}
}

// Configure 配置格式化器模块
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

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.BaseModule.Configure(config); err != nil {
		return err
	}
	f.formatters = next
	return nil
}

// FormatterFunctions 实现 modules.FormatterProvider，避免反射与 interface{} 转换。
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

// init 注册formatter模块工厂
func init() {
	if err := modules.RegisterFactory("formatter", func(config modules.Config) (modules.Module, error) {
		adapter := NewFormatterAdapter()
		return adapter, adapter.Configure(config)
	}); err != nil {
		modules.ReportAsyncError("registry.formatter", err)
	}
}
