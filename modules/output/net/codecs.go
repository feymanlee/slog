package outputnet

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/feymanlee/slog/internal/common"
)

// Codec converts slog records into bytes for network transport.
type Codec interface {
	Name() string
	Encode(record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error)
}

var globalCodecs = common.NewNamedRegistry[Codec]()

func init() {
	_ = RegisterCodec(rawCodec{})
	_ = RegisterCodec(jsonCodec{})
}

// RegisterCodec registers a codec by name.
func RegisterCodec(codec Codec) error {
	if codec == nil || strings.TrimSpace(codec.Name()) == "" {
		return errInvalidCodec
	}
	name := strings.ToLower(strings.TrimSpace(codec.Name()))
	globalCodecs.Set(name, codec)
	return nil
}

// GetCodec returns a registered codec. Empty name defaults to raw.
func GetCodec(name string) (Codec, bool) {
	return globalCodecs.Get(name, "raw")
}

type rawCodec struct{}

func (c rawCodec) Name() string { return "raw" }

func (c rawCodec) Encode(record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error) {
	if record == nil {
		return []byte("level=INFO msg="), nil
	}
	allAttrs := common.AppendRecordAttrsToAttrs(attrs, groups, record)
	allAttrs = common.RemoveEmptyAttrs(allAttrs)

	var buf bytes.Buffer
	buf.WriteString("level=")
	buf.WriteString(record.Level.String())
	buf.WriteString(" msg=")
	buf.WriteString(record.Message)
	for _, attr := range allAttrs {
		buf.WriteByte(' ')
		buf.WriteString(attr.Key)
		buf.WriteByte('=')
		buf.WriteString(attr.Value.String())
	}
	return buf.Bytes(), nil
}

type jsonCodec struct{}

func (c jsonCodec) Name() string { return "json" }

func (c jsonCodec) Encode(record *slog.Record, attrs []slog.Attr, groups []string) ([]byte, error) {
	return json.Marshal(common.JSONRecordPayload(record, attrs, groups, true))
}
