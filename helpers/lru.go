package helpers

import (
	"unsafe"

	"github.com/elastic/go-freelru"
	"github.com/orisano/wyhash"
)

type LRU struct {
	cache *freelru.ShardedLRU[string, string]
}

type LRUInterface interface {
	Set(key, value string)
	Get(key string) string
}

var _ LRUInterface = (*LRU)(nil)

func hashString(s string) uint32 {
	return uint32(wyhash.Sum64(0, unsafe.Slice(unsafe.StringData(s), len(s))))
}

func NewLRU() *LRU {
	capacity := uint32(2048)
	cache, err := freelru.NewSharded[string, string](capacity, hashString)
	if err != nil {
		return nil
	}
	return &LRU{cache: cache}
}

func (lru *LRU) Set(key, value string) {
	// now := time.Now()
	lru.cache.Add(key, value)
	// log.Infof("set time: %v", time.Since(now))
}

func (lru *LRU) Get(key string) string {
	// now := time.Now()
	if v, ok := lru.cache.Peek(key); ok {
		// log.Infof("get time: %v", time.Since(now))
		return v
	}
	return ""
}
