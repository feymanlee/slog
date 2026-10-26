package slog

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Err 将错误构造成带 message、type 和 stacktrace 的 error 属性。
func Err(err error) Attr {
	if err == nil {
		return Attr{}
	}
	return Group("error",
		String("message", err.Error()),
		String("type", fmt.Sprintf("%T", err)),
		String("stacktrace", errorStack()),
	)
}

func errorStack() string {
	lines := strings.Split(string(debug.Stack()), "\n")
	if len(lines) < 3 {
		return strings.Join(lines, "\n")
	}

	for i := 1; i+1 < len(lines); i += 2 {
		if isSlogStackFrame(lines[i]) {
			continue
		}
		return strings.Join(append([]string{lines[0]}, lines[i:]...), "\n")
	}
	return strings.Join(lines, "\n")
}

func isSlogStackFrame(function string) bool {
	if strings.Contains(function, "github.com/feymanlee/slog.Test") {
		return false
	}
	return strings.Contains(function, "github.com/feymanlee/slog.") ||
		strings.Contains(function, "log/slog.") ||
		strings.HasPrefix(function, "runtime/")
}
