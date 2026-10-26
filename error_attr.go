package slog

import (
	"fmt"
	stdslog "log/slog"
	"runtime/debug"
	"strings"
)

// errAttr expands an error attribute into searchable error details and a stack trace.
func errAttr(attr stdslog.Attr) stdslog.Attr {
	if attr.Key != "error" {
		return attr
	}

	value := attr.Value.Resolve()
	raw := value.Any()
	if raw == nil {
		return stdslog.Attr{}
	}
	err, ok := raw.(error)
	if !ok || err == nil {
		return attr
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

func normalizeErrorAttrs(attrs []stdslog.Attr) []stdslog.Attr {
	if len(attrs) == 0 {
		return attrs
	}
	normalized := make([]stdslog.Attr, len(attrs))
	for i, attr := range attrs {
		normalized[i] = errAttr(attr)
	}
	return normalized
}
