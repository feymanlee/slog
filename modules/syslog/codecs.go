package syslog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	svr "github.com/feymanlee/slog"
	"github.com/feymanlee/slog/internal/common"
)

var errInvalidCodec = errors.New("syslog: invalid codec")

// Codec converts slog records to payload bytes (without CEE prefix).
type Codec interface {
	Name() string
	Encode(ctx context.Context, record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error)
}

var globalCodecs = common.NewNamedRegistry[Codec]()

func init() {
	_ = RegisterCodec(defaultCodec{})
	_ = RegisterCodec(jsonCodec{})
}

func RegisterCodec(codec Codec) error {
	if codec == nil || strings.TrimSpace(codec.Name()) == "" {
		return errInvalidCodec
	}
	name := strings.ToLower(strings.TrimSpace(codec.Name()))
	globalCodecs.Set(name, codec)
	return nil
}

func GetCodec(name string) (Codec, bool) {
	return globalCodecs.Get(name, "default")
}

type defaultCodec struct{}

func (c defaultCodec) Name() string { return "default" }

func (c defaultCodec) Encode(_ context.Context, record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error) {
	attrs = common.AppendRecordAttrsToAttrs(attrs, groups, record)
	attrs = common.ReplaceError(attrs, defaultErrorKeys...)
	attrs = common.RemoveEmptyAttrs(attrs)

	payload := map[string]any{
		"logger.name":     svr.Name,
		"logger.version":  svr.Version,
		"timestamp":       record.Time.UTC(),
		"level":           record.Level.String(),
		"message":         record.Message,
		defaultContextKey: common.AttrsToMap(attrs...),
	}
	return json.Marshal(payload)
}

type jsonCodec struct{}

func (c jsonCodec) Name() string { return "json" }

func (c jsonCodec) Encode(_ context.Context, record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error) {
	return json.Marshal(common.JSONRecordPayload(record, attrs, groups, false))
}

var (
	defaultContextKey = "extra"
	defaultErrorKeys  = []string{"error", "err"}
)
