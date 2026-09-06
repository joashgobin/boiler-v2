package helpers

import (
	"time"
	"unsafe"

	"github.com/elastic/go-freelru"
	"github.com/orisano/wyhash"
)

type LRU[T any] struct {
	cache *freelru.ShardedLRU[string, T]
}

func hashString(s string) uint32 {
	return uint32(wyhash.Sum64(0, unsafe.Slice(unsafe.StringData(s), len(s))))
}

func NewLRU[T any](exampleValue T, lifetime ...time.Duration) *LRU[T] {
	capacity := uint32(128)
	cache, err := freelru.NewSharded[string, T](capacity, hashString)
	if err != nil {
		return nil
	}
	if len(lifetime) > 0 {
		cache.SetLifetime(lifetime[0])
	}
	return &LRU[T]{cache: cache}
}

func (lru *LRU[T]) Set(key string, value T) {
	// now := time.Now()
	lru.cache.Add(key, value)
	// log.Infof("set time: %v", time.Since(now))
}

func (lru *LRU[T]) Get(key string) T {
	var defaultValue T
	// now := time.Now()
	if v, ok := lru.cache.Peek(key); ok {
		// log.Infof("get time: %v", time.Since(now))
		return v
	}
	return defaultValue
}
