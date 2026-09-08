package modules

import "log/slog"

// FormatterProvider 提供格式化函数，避免使用反射适配。
type FormatterProvider interface {
	FormatterFunctions() []func([]string, slog.Attr) (slog.Value, bool)
}
