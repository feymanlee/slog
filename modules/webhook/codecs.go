package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	svr "github.com/feymanlee/slog"
	"github.com/feymanlee/slog/internal/common"
)

var errInvalidCodec = errors.New("webhook: invalid codec")

// Codec converts slog records to HTTP payload bytes.
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

	extra := common.AttrsToMap(attrs...)
	payload := map[string]any{
		"logger.name":    svr.Name,
		"logger.version": svr.Version,
		"timestamp":      record.Time.UTC(),
		"level":          record.Level.String(),
		"message":        record.Message,
	}

	for _, errorKey := range defaultErrorKeys {
		if v, ok := extra[errorKey]; ok {
			if err, ok := v.(error); ok {
				payload[errorKey] = common.FormatError(err)
				delete(extra, errorKey)
				break
			}
		}
	}

	if v, ok := extra[defaultRequestKey]; ok {
		if req, ok := v.(*http.Request); ok {
			payload[defaultRequestKey] = common.FormatRequest(req, defaultRequestIgnoreHeaders)
			delete(extra, defaultRequestKey)
		}
	}
	if user, ok := extra["user"]; ok {
		payload["user"] = user
		delete(extra, "user")
	}
	payload[defaultContextKey] = extra

	return json.Marshal(payload)
}

type jsonCodec struct{}

func (c jsonCodec) Name() string { return "json" }

func (c jsonCodec) Encode(_ context.Context, record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error) {
	return json.Marshal(common.JSONRecordPayload(record, attrs, groups, false))
}

var (
	defaultContextKey           = "extra"
	defaultErrorKeys            = []string{"error", "err"}
	defaultRequestKey           = "request"
	defaultRequestIgnoreHeaders = false
)
