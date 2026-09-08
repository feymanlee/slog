package common

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestExtractFromContextHandlesNonStringKeysAndNilContext(t *testing.T) {
	type requestIDKey struct{}

	key := requestIDKey{}
	ctx := context.WithValue(context.Background(), key, "req-1")
	extractor := ExtractFromContext("trace_id", key)

	attrs := extractor(ctx)
	if len(attrs) != 2 {
		t.Fatalf("expected 2 attrs, got %d", len(attrs))
	}
	if attrs[1].Key != fmt.Sprint(key) || attrs[1].Value.String() != "req-1" {
		t.Fatalf("unexpected non-string key attr: %+v", attrs[1])
	}

	nilAttrs := ExtractFromContext("trace_id")(nil)
	if len(nilAttrs) != 1 || nilAttrs[0].Value.Kind() != slog.KindAny {
		t.Fatalf("nil context should produce a stable nil attr, got %+v", nilAttrs)
	}
}

// TestLRUCache_AdditionalEdgeCases 测试LRU缓存的边界情况
func TestLRUCache_AdditionalEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "零容量缓存",
			test: func(t *testing.T) {
				cache := NewLRUCache(0)
				// 零容量会被设置为默认容量100，所以应该能存储内容
				stats := cache.GetStats()
				if stats.Capacity != 100 {
					t.Errorf("零容量应该被设置为默认值100，实际: %d", stats.Capacity)
				}
			},
		},
		{
			name: "负容量缓存",
			test: func(t *testing.T) {
				cache := NewLRUCache(-1)
				// 负容量会被设置为默认容量100
				stats := cache.GetStats()
				if stats.Capacity != 100 {
					t.Errorf("负容量应该被设置为默认值100，实际: %d", stats.Capacity)
				}
			},
		},
		{
			name: "大容量缓存",
			test: func(t *testing.T) {
				cache := NewLRUCache(10000)

				// 添加大量数据
				for i := range 1000 {
					cache.Put(i, i*2)
				}

				// 验证数据存在
				for i := range 1000 {
					if value, exists := cache.Get(i); !exists || value != i*2 {
						t.Errorf("大容量缓存数据丢失: key=%d", i)
					}
				}
			},
		},
		{
			name: "nil值存储",
			test: func(t *testing.T) {
				cache := NewLRUCache(5)
				cache.Put("nil_key", nil)

				if value, exists := cache.Get("nil_key"); !exists || value != nil {
					t.Error("应该能够存储nil值")
				}
			},
		},
		{
			name: "相同键多次Put",
			test: func(t *testing.T) {
				cache := NewLRUCache(3)

				cache.Put("key", "value1")
				cache.Put("key", "value2")
				cache.Put("key", "value3")

				if value, exists := cache.Get("key"); !exists || value != "value3" {
					t.Error("相同键的最后值应该被保留")
				}

				if cache.Size() != 1 {
					t.Errorf("缓存大小应该为1，实际: %d", cache.Size())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestLRUCache_Statistics 测试缓存统计功能
func TestLRUCache_Statistics(t *testing.T) {
	cache := NewLRUCache(3)

	// 初始统计
	stats := cache.GetStats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("初始统计应该全为零")
	}

	// 添加数据并访问
	cache.Put("a", 1)
	cache.Put("b", 2)
	cache.Put("c", 3)

	// 命中测试
	cache.Get("a")
	cache.Get("b")
	cache.Get("a") // 再次命中

	// 未命中测试
	cache.Get("d")
	cache.Get("e")

	// 导致淘汰
	cache.Put("d", 4) // 应该淘汰c

	stats = cache.GetStats()
	if stats.Hits != 3 {
		t.Errorf("期望3次命中，实际: %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("期望2次未命中，实际: %d", stats.Misses)
	}
	// 注意：LRUCacheStats没有Evictions字段，所以我们检查其他指标
}

// TestLRUCache_ClearAndSize 测试清除和大小功能
func TestLRUCache_ClearAndSize(t *testing.T) {
	cache := NewLRUCache(5)

	// 添加数据
	for i := range 3 {
		cache.Put(i, i*2)
	}

	if cache.Size() != 3 {
		t.Errorf("期望大小为3，实际: %d", cache.Size())
	}

	// 清除缓存
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("清除后大小应该为0，实际: %d", cache.Size())
	}

	// 验证数据确实被清除
	for i := range 3 {
		if _, exists := cache.Get(i); exists {
			t.Errorf("清除后数据不应该存在: key=%d", i)
		}
	}

	// 验证统计被重置
	stats := cache.GetStats()
	if stats.Hits != 0 || stats.Misses != 3 { // 刚才的Get调用产生了3次miss
		t.Error("清除后统计应该正确")
	}
}

// TestLRUCache_ThreadSafety 测试线程安全
func TestLRUCache_ThreadSafety(t *testing.T) {
	cache := NewLRUCache(100)
	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	start := make(chan struct{})

	// 并发写入
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for j := range numOperations {
				key := id*numOperations + j
				cache.Put(key, key*2)
			}
		}(i)
	}

	// 并发读取
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for j := range numOperations {
				key := id*numOperations + j
				cache.Get(key)
			}
		}(i)
	}

	// 并发清除和统计
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		cache.Clear()
	}()

	go func() {
		defer wg.Done()
		<-start
		for range 50 {
			cache.GetStats()
		}
	}()
	close(start)

	// 等待所有goroutine完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 测试完成
	case <-time.After(5 * time.Second):
		t.Fatal("并发测试超时")
	}
}

// TestStringCache_Coverage 测试字符串缓存覆盖
func TestStringCache_Coverage(t *testing.T) {
	cache := NewLRUCache(5)

	// 测试基本操作
	cache.Put("key1", "value1")
	cache.Put("key2", "value2")

	if value, exists := cache.Get("key1"); !exists || value != "value1" {
		t.Error("字符串缓存获取失败")
	}

	if _, exists := cache.Get("nonexistent"); exists {
		t.Error("不存在的键不应该返回true")
	}

	// 测试容量限制
	for i := 3; i <= 10; i++ {
		cache.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
	}

	if cache.Size() > 5 {
		t.Errorf("缓存大小不应该超过容量限制，实际: %d", cache.Size())
	}

	// 测试LRU行为
	cache.Get("key6")                 // 访问key6使其成为最近使用
	cache.Put("new_key", "new_value") // 这应该淘汰除key6之外的某个键

	if _, exists := cache.Get("key6"); !exists {
		t.Error("最近访问的key6应该仍然存在")
	}
}
