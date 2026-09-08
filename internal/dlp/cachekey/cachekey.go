package cachekey

import (
	"strconv"

	"github.com/feymanlee/slog/internal/xxhash"
)

// CacheKey 缓存键优化器
type CacheKey struct {
	prefixCache map[string]uint64
}

// New 创建缓存键优化器
func New() *CacheKey {
	return &CacheKey{
		prefixCache: make(map[string]uint64),
	}
}

// GenerateKey 生成优化的缓存键
// 使用 xxhash 算法生成高性能、低碰撞的缓存键
func (cko *CacheKey) GenerateKey(prefix, data string) string {
	if len(data) <= 32 {
		return prefix + ":" + data
	}

	hash := xxhash.Sum64String(data)
	return prefix + ":h" + strconv.FormatUint(hash, 16) + ":" + strconv.Itoa(len(data))
}

// GenerateKeyWithContext 生成带上下文的缓存键
func (cko *CacheKey) GenerateKeyWithContext(desensitizer, dataType, data string) string {
	if len(data) <= 32 {
		return desensitizer + ":" + dataType + ":" + data
	}

	contextHash := cko.getOrComputeContextHash(desensitizer + ":" + dataType)
	dataHash := xxhash.Sum64String(data)
	combinedHash := contextHash ^ dataHash

	return "ch:" + strconv.FormatUint(combinedHash, 16) + ":" + strconv.Itoa(len(data))
}

func (cko *CacheKey) getOrComputeContextHash(context string) uint64 {
	if hash, exists := cko.prefixCache[context]; exists {
		return hash
	}

	hash := xxhash.Sum64String(context)

	if len(cko.prefixCache) < 100 {
		cko.prefixCache[context] = hash
	}

	return hash
}

// GenerateFastKey 生成快速键（平衡性能和碰撞率）
func (cko *CacheKey) GenerateFastKey(data string) string {
	dataLen := len(data)

	if dataLen <= 32 {
		return data
	}

	prefixLen := min(dataLen, 8)

	middlePart := data[prefixLen:]
	hash := xxhash.Sum64String(middlePart)

	return data[:prefixLen] + "h" + strconv.FormatUint(hash, 16) + ":" + strconv.Itoa(dataLen)
}

// 全局缓存键优化器实例（仅供内部模块复用）
var Global = New()

// Key 全局键生成函数
func Key(prefix, data string) string {
	return Global.GenerateKey(prefix, data)
}

// KeyWithContext 全局上下文键生成函数
func KeyWithContext(desensitizer, dataType, data string) string {
	return Global.GenerateKeyWithContext(desensitizer, dataType, data)
}

// FastKey 全局快速键生成函数
func FastKey(data string) string {
	return Global.GenerateFastKey(data)
}
