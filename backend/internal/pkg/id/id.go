package id

import (
	"crypto/rand"
	"encoding/hex"
)

// NewChunkUID 生成 chunk 在 MySQL 和 Milvus 之间共用的稳定主键。
func NewChunkUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "chunk_unknown"
	}
	return "chunk_" + hex.EncodeToString(b[:])
}
