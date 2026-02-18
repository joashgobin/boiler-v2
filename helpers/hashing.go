package helpers

import (
	"crypto/sha256"
	"encoding/hex"
)

func FingerprintFromBuffer(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func GetHash(content string) string {
	hashBytes := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hashBytes[:])
}
