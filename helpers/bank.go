package helpers

import (
	"encoding/json"
	"time"

	"github.com/gofiber/storage/valkey"
)

type Bank struct {
	storage *valkey.Storage
	prefix  string
}

type BankInterface interface {
	GetString(key string) string
	SetString(key string, value string, exp time.Duration)
	Delete(key string)
	GetBytes(key string) []byte
	SetBytes(key string, value []byte, exp time.Duration)
	Close()
}

var _ BankInterface = (*Bank)(nil)

func NewBank(storage *valkey.Storage, appName string) *Bank {
	return &Bank{storage: storage, prefix: appName + "-"}
}

func (b *Bank) Close() {
	b.storage.Close()
}

func (b *Bank) GetString(key string) string {
	data, err := b.storage.Get(b.prefix + key)
	if err != nil {
		return ""
	}
	return string(data)
}

func (b *Bank) Delete(key string) {
	b.storage.Delete(b.prefix + key)
}

func (b *Bank) SetString(key string, value string, exp time.Duration) {
	b.storage.Set(b.prefix+key, []byte(value), exp)
}

func (b *Bank) GetBytes(key string) []byte {
	data, err := b.storage.Get(b.prefix + key)
	if err != nil {
		return []byte{}
	}
	return data
}

func (b *Bank) SetBytes(key string, value []byte, exp time.Duration) {
	b.storage.Set(b.prefix+key, value, exp)
}

func SliceToBytes[T any](data []T) []byte {
	jsonStr, err := json.Marshal(data)
	if err != nil {
		return []byte{}
	}
	return jsonStr
}

func BytesToSlice[T any](bytes []byte) []T {
	var decoded []T
	err := json.Unmarshal(bytes, &decoded)
	if err != nil {
		return []T{}
	}
	return decoded
}
