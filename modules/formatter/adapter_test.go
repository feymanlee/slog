package formatter

import (
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/darkit/slog/modules"
)

func TestFormatterAdapterConfigureReplacesPreviousFormatters(t *testing.T) {
	adapter := NewFormatterAdapter()
	if err := adapter.Configure(modules.Config{"type": "error", "replacement": "error"}); err != nil {
		t.Fatalf("configure initial formatter: %v", err)
	}
	if err := adapter.Configure(modules.Config{"type": "error", "replacement": "err"}); err != nil {
		t.Fatalf("replace formatter: %v", err)
	}

	funcs := adapter.FormatterFunctions()
	if len(funcs) != 1 {
		t.Fatalf("expected one replacement formatter, got %d", len(funcs))
	}
	if _, ok := funcs[0](nil, slog.Any("error", errors.New("old"))); ok {
		t.Fatal("expected previous error formatter to be removed")
	}
	if _, ok := funcs[0](nil, slog.Any("err", errors.New("current"))); !ok {
		t.Fatal("expected replacement formatter to handle err")
	}
}

func TestFormatterAdapterConfigureFailureKeepsPreviousFormatters(t *testing.T) {
	adapter := NewFormatterAdapter()
	if err := adapter.Configure(modules.Config{"type": "error", "replacement": "error"}); err != nil {
		t.Fatalf("configure initial formatter: %v", err)
	}
	if err := adapter.Configure(modules.Config{"type": make(chan int)}); err == nil {
		t.Fatal("expected invalid config to fail")
	}

	funcs := adapter.FormatterFunctions()
	if len(funcs) != 1 {
		t.Fatalf("expected initial formatter to survive, got %d", len(funcs))
	}
	if _, ok := funcs[0](nil, slog.Any("error", errors.New("still active"))); !ok {
		t.Fatal("expected initial formatter to remain active")
	}
}

func TestFormatterAdapterConfigureConcurrent(t *testing.T) {
	adapter := NewFormatterAdapter()
	configs := []modules.Config{
		{"type": "time", "format": "2006-01-02"},
		{"type": "error", "replacement": "err"},
	}

	const workers = 8
	const iterations = 100
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				if err := adapter.Configure(configs[(worker+i)%len(configs)]); err != nil {
					t.Errorf("configure: %v", err)
					return
				}
			}
		}(worker)
	}

	close(start)
	wg.Wait()
}
