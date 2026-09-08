package common

import (
	"log/slog"
	"maps"
	"time"
)

// JSONRecordPayload builds the common flat JSON representation for a record.
func JSONRecordPayload(record *slog.Record, attrs []slog.Attr, groups []string, removeEmpty bool) map[string]any {
	if record == nil {
		return map[string]any{"level": "INFO", "message": ""}
	}
	flatAttrs := AppendRecordAttrsToAttrs(attrs, groups, record)
	if removeEmpty {
		flatAttrs = RemoveEmptyAttrs(flatAttrs)
	}
	flat := AttrsToMap(flatAttrs...)
	payload := map[string]any{
		"level":   record.Level.String(),
		"message": record.Message,
	}
	if !record.Time.IsZero() {
		payload["timestamp"] = record.Time.UTC().Format(time.RFC3339Nano)
	}
	maps.Copy(payload, flat)
	return payload
}
