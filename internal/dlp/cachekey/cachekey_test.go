package cachekey

import (
	"strings"
	"testing"
)

func TestCacheKeyGenerateKey(t *testing.T) {
	optimizer := New()
	short := optimizer.GenerateKey("phone", "13812345678")
	if short != "phone:13812345678" {
		t.Fatalf("short key = %q", short)
	}

	long := optimizer.GenerateKey("text", strings.Repeat("long text content", 10))
	if !strings.HasPrefix(long, "text:h") || !strings.Contains(long, ":") {
		t.Fatalf("long key = %q", long)
	}
}

func TestCacheKeyGenerateKeyWithContext(t *testing.T) {
	optimizer := New()
	data := strings.Repeat("test@example.com", 3)
	key := optimizer.GenerateKeyWithContext("email", "email_address", data)
	if key != optimizer.GenerateKeyWithContext("email", "email_address", data) {
		t.Fatal("same input should produce the same key")
	}
	if key == optimizer.GenerateKeyWithContext("phone", "email_address", data) {
		t.Fatal("different context should produce a different key")
	}
}

func TestCacheKeyGenerateFastKey(t *testing.T) {
	optimizer := New()
	short := "short data"
	if got := optimizer.GenerateFastKey(short); got != short {
		t.Fatalf("short key = %q", got)
	}

	data := strings.Repeat("fast key benchmark", 25)
	key := optimizer.GenerateFastKey(data)
	if !strings.HasPrefix(key, data[:8]+"h") || !strings.HasSuffix(key, ":450") {
		t.Fatalf("fast key = %q", key)
	}
}

func TestCacheKeyDeterministic(t *testing.T) {
	optimizer := New()
	data := strings.Repeat("cache key test", 20)
	if got, want := optimizer.GenerateKey("prefix", data), optimizer.GenerateKey("prefix", data); got != want {
		t.Fatalf("keys differ: %q != %q", got, want)
	}
}
