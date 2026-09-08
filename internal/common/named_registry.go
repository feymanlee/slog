package common

import (
	"strings"
	"sync"
)

// NamedRegistry stores values under normalized names.
type NamedRegistry[T any] struct {
	mu     sync.RWMutex
	values map[string]T
}

func NewNamedRegistry[T any]() *NamedRegistry[T] {
	return &NamedRegistry[T]{values: make(map[string]T)}
}

func (r *NamedRegistry[T]) Set(name string, value T) {
	r.mu.Lock()
	r.values[normalizeName(name)] = value
	r.mu.Unlock()
}

func (r *NamedRegistry[T]) Get(name, fallback string) (T, bool) {
	if strings.TrimSpace(name) == "" {
		name = fallback
	}
	r.mu.RLock()
	value, ok := r.values[normalizeName(name)]
	r.mu.RUnlock()
	return value, ok
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
