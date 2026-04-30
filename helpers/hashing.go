package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/zeebo/xxh3"
)

func FingerprintFromBuffer(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func GetHash(content string) string {
	hashBytes := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hashBytes[:])
}

func GetXXH3(content string) string {
	return strconv.FormatUint(xxh3.HashString(content), 16)
}
