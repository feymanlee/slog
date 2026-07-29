package multi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type handlerStub struct {
	enabled   bool
	handle    func(slog.Record) error
	withAttrs func([]slog.Attr) slog.Handler
	withGroup func(string) slog.Handler
}

func (h *handlerStub) Enabled(context.Context, slog.Level) bool { return h.enabled }

func (h *handlerStub) Handle(_ context.Context, record slog.Record) error {
	if h.handle == nil {
		return nil
	}
	return h.handle(record)
}

func (h *handlerStub) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h.withAttrs == nil {
		return h
	}
	return h.withAttrs(attrs)
}

func (h *handlerStub) WithGroup(name string) slog.Handler {
	if h.withGroup == nil {
		return h
	}
	return h.withGroup(name)
}

func TestFanoutDeliversIndependentRecordsToEnabledHandlers(t *testing.T) {
	var secondAttrs []slog.Attr
	first := &handlerStub{enabled: true, handle: func(record slog.Record) error {
		record.AddAttrs(slog.String("private", "first"))
		return nil
	}}
	second := &handlerStub{enabled: true, handle: func(record slog.Record) error {
		record.Attrs(func(attr slog.Attr) bool {
			secondAttrs = append(secondAttrs, attr)
			return true
		})
		return nil
	}}
	disabledCalled := false
	disabled := &handlerStub{enabled: false, handle: func(slog.Record) error {
		disabledCalled = true
		return nil
	}}

	record := slog.NewRecord(time.Unix(1, 0), slog.LevelInfo, "fanout", 0)
	record.AddAttrs(slog.String("request_id", "r1"))
	if err := Fanout(first, disabled, second).Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if disabledCalled {
		t.Fatal("disabled handler received a record")
	}
	if len(secondAttrs) != 1 || secondAttrs[0].Key != "request_id" || secondAttrs[0].Value.String() != "r1" {
		t.Fatalf("second handler attrs = %v, want only request_id=r1", secondAttrs)
	}
}

func TestFanoutJoinsErrorsAndContinuesAfterPanics(t *testing.T) {
	firstErr := errors.New("first failed")
	panicErr := errors.New("panic failed")
	lastCalled := false
	handler := Fanout(
		&handlerStub{enabled: true, handle: func(slog.Record) error { return firstErr }},
		&handlerStub{enabled: true, handle: func(slog.Record) error { panic(panicErr) }},
		&handlerStub{enabled: true, handle: func(slog.Record) error { panic("bad state") }},
		&handlerStub{enabled: true, handle: func(slog.Record) error {
			lastCalled = true
			return nil
		}},
	)

	err := handler.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "fanout", 0))
	if !errors.Is(err, firstErr) || !errors.Is(err, panicErr) {
		t.Fatalf("Handle() error = %v, want both downstream errors", err)
	}
	if !strings.Contains(err.Error(), "unexpected error: bad state") {
		t.Fatalf("Handle() error = %v, want converted non-error panic", err)
	}
	if !lastCalled {
		t.Fatal("handler after panic was not called")
	}
}

func TestFanoutEnabledWhenAnyHandlerAcceptsLevel(t *testing.T) {
	handler := Fanout(&handlerStub{enabled: false}, &handlerStub{enabled: true})
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("Enabled() = false, want true when one downstream handler is enabled")
	}

	disabled := Fanout(&handlerStub{enabled: false}, &handlerStub{enabled: false})
	if disabled.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("Enabled() = true, want false when all downstream handlers are disabled")
	}
}

func TestFanoutDerivationPropagatesIndependentAttrsAndGroups(t *testing.T) {
	var secondAttrs []slog.Attr
	var groups []string
	first := &handlerStub{enabled: true}
	first.withAttrs = func(attrs []slog.Attr) slog.Handler {
		attrs[0] = slog.String("changed", "first")
		return first
	}
	first.withGroup = func(name string) slog.Handler {
		groups = append(groups, "first:"+name)
		return first
	}
	second := &handlerStub{enabled: true}
	second.withAttrs = func(attrs []slog.Attr) slog.Handler {
		secondAttrs = append(secondAttrs, attrs...)
		return second
	}
	second.withGroup = func(name string) slog.Handler {
		groups = append(groups, "second:"+name)
		return second
	}

	original := []slog.Attr{slog.String("request_id", "r1")}
	derived := Fanout(first, second).WithAttrs(original)
	if len(secondAttrs) != 1 || secondAttrs[0].Key != "request_id" {
		t.Fatalf("second handler attrs = %v, want an unmodified copy", secondAttrs)
	}
	if original[0].Key != "request_id" {
		t.Fatalf("caller attrs were mutated: %v", original)
	}

	if got := derived.WithGroup(""); got != derived {
		t.Fatal("WithGroup(\"\") should return the same handler")
	}
	derived.WithGroup("request")
	if len(groups) != 2 || groups[0] != "first:request" || groups[1] != "second:request" {
		t.Fatalf("propagated groups = %v", groups)
	}
}
