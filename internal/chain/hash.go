package chain

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/cespare/xxhash/v2"

	"auditlog/internal/model"
)

// BlockHash 计算日志块的完整哈希：包含块序号、创建时间与全部记录。
func BlockHash(block *model.Block) uint64 {
	digest := xxhash.New()
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], block.ID)
	_, _ = digest.Write(scratch[:])
	_, _ = digest.Write(TimeBytes(block.CreatedAt))
	for _, rec := range block.Records {
		_, _ = digest.Write(RecordsHashBytes(rec))
	}
	return digest.Sum64()
}

// RecordsHash 计算记录数组的摘要，用于跨块校验记录连续性。
func RecordsHash(records []model.Record) uint64 {
	digest := xxhash.New()
	for _, rec := range records {
		_, _ = digest.Write(RecordsHashBytes(rec))
	}
	return digest.Sum64()
}

// RecordsHashBytes 返回单条记录用于哈希的规范字节。
func RecordsHashBytes(rec model.Record) []byte {
	encoded := rec.Encode()
	return encoded
}

// TimeBytes 将时间转换为用于哈希的规范字节。
func TimeBytes(at time.Time) []byte {
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], uint64(at.UnixNano()))
	return scratch[:]
}

// DigestHex 将哈希值格式化为十六进制字符串，供日志与页面展示。
func DigestHex(value uint64) string {
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], value)
	return fmt.Sprintf("%016x", value)
}
