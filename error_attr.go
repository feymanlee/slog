package slog

import (
	"fmt"
	stdslog "log/slog"
	"runtime/debug"
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
		String("stacktrace", string(debug.Stack())),
	)
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
