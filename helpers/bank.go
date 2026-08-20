package helpers

import (
	"bytes"
	"encoding/gob"
	"encoding/json/v2"
	"time"

	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/storage/valkey"
)

type Bank struct {
	storage *valkey.Storage
	prefix  string
}

/*
type BankInterface interface {
	GetString(key string) string
	SetString(key string, value string, exp time.Duration)
	Delete(key string)
	GetBytes(key string) []byte
	SetBytes(key string, valueAsBytes []byte, exp time.Duration)
	Close()
}

var _ BankInterface = (*Bank)(nil)

type BankAdaptorInterface[T any] interface {
	Get(key string) T
	Set(key string, value T, exp time.Duration)
	GetSlice(key string) []T
	SetSlice(key string, value []T, exp time.Duration)
}

type BankAdaptor[T any] struct {
	bank BankInterface
}

func NewBankAdaptor[T any](bank BankInterface) *BankAdaptor[T] {
	return &BankAdaptor[T]{bank: bank}
}

func (ba *BankAdaptor[T]) Get(key string) T {
	bytes := ba.bank.GetBytes(key)
	return FromBytes[T](bytes)
}

func (ba *BankAdaptor[T]) Set(key string, value T, exp time.Duration) {
	ba.bank.SetBytes(key, ToBytes(value), exp)
}

func (ba *BankAdaptor[T]) GetSlice(key string) []T {
	bytes := ba.bank.GetBytes(key)
	slice := BytesToSlice[T](bytes)
	return slice
}

func (ba *BankAdaptor[T]) SetSlice(key string, value []T, exp time.Duration) {
	bytes := SliceToBytes[T](value)
	ba.bank.SetBytes(key, bytes, exp)
}
*/

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

func (b *Bank) SetBytes(key string, valueAsBytes []byte, exp time.Duration) {
	b.storage.Set(b.prefix+key, valueAsBytes, exp)
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

func ToBytes(p any) []byte {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(p)
	if err != nil {
		log.Errorf("to bytes error: %v", err)
	}
	// fmt.Println("uncompressed size (bytes): ", len(buf.Bytes()))
	return buf.Bytes()
}

func FromBytes[T any](s []byte) T {
	p := new(T)
	dec := gob.NewDecoder(bytes.NewReader(s))
	err := dec.Decode(&p)
	if err != nil {
		log.Errorf("to struct error: %v", err)
	}
	return *p
}

func (b *Bank) Get[T any](key string, defaultValue T) T {
	data := b.GetBytes(key)
	if len(data) == 0 {
		return defaultValue
	}
	p := new(T)
	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&p)
	if err != nil {
		return defaultValue
	}
	return *p
}

func (b *Bank) GetSlice[T any](key string) []T {
	data := b.GetBytes(key)
	if len(data) == 0 {
		return []T{}
	}
	var decoded []T
	err := json.Unmarshal(data, &decoded)
	if err != nil {
		return []T{}
	}
	return decoded
}

func (b *Bank) Set[T any](key string, value T, exp time.Duration) {
	b.SetBytes(key, ToBytes(value), exp)
}

func (b *Bank) SetSlice[T any](key string, value []T, exp time.Duration) {
	bytes := SliceToBytes[T](value)
	b.SetBytes(key, bytes, exp)
}
