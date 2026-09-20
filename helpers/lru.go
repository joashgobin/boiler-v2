package helpers

import (
	"hash/maphash"
	"time"
	"unsafe"

	"github.com/elastic/go-freelru"
	"github.com/orisano/wyhash"
)

type LRU[T any] struct {
	cache *freelru.ShardedLRU[string, T]
}

type FullLRU[T comparable, U any] struct {
	cache *freelru.ShardedLRU[T, U]
}

func hashString(s string) uint32 {
	return uint32(wyhash.Sum64(0, unsafe.Slice(unsafe.StringData(s), len(s))))
}

func hashGeneric[K comparable]() func(K) uint32 {
	seed := maphash.MakeSeed()
	hashFunc := func(key K) uint32 {
		switch k := any(key).(type) {
		case int:
			return uint32(k)
		case uint32:
			return k
		case string:
			return hashString(k)
		default:
			return uint32(maphash.Comparable(seed, key))
		}
	}
	return hashFunc
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

func (lru *LRU[T]) Clear() {
	lru.cache.Purge()
}

func NewFullLRU[T comparable, U any](exampleKey T, exampleValue U, lifetime ...time.Duration) *FullLRU[T, U] {
	capacity := uint32(128)
	cache, err := freelru.NewSharded[T, U](capacity, hashGeneric[T]())
	if err != nil {
		return nil
	}
	if len(lifetime) > 0 {
		cache.SetLifetime(lifetime[0])
	}
	return &FullLRU[T, U]{cache: cache}
}

func (lru *FullLRU[T, U]) Set(key T, value U) {
	lru.cache.Add(key, value)
}

func (lru *FullLRU[T, U]) Get(key T) U {
	var defaultValue U
	if v, ok := lru.cache.Peek(key); ok {
		return v
	}
	return defaultValue
}

func (lru *FullLRU[T, U]) Clear() {
	lru.cache.Purge()
}
