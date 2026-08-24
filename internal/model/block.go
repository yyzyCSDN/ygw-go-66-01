package model

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// State 描述日志块的生命周期状态。
type State string

const (
	StateActive   State = "active"
	StateArchived State = "archived"
	StateExpired  State = "expired"
)

// Block 是一批不可变日志记录。PrevHash 链接上一块的哈希，Hash 为本块哈希，
// 两者共同构成哈希链。File 记录块文件相对数据目录的路径。
type Block struct {
	ID        uint64
	PrevHash  uint64
	Hash      uint64
	CreatedAt time.Time
	Records   []Record
	State     State
	File      string
}

const blockMagic = 0x4155444c // "AUDL"

// Encode 将块序列化为二进制：固定头部加记录区。
func (b *Block) Encode() []byte {
	header := 48 + len(b.State)
	records := EncodeRecords(b.Records)
	buf := make([]byte, header+len(records))
	off := 0
	binary.BigEndian.PutUint32(buf[off:], blockMagic)
	off += 4
	binary.BigEndian.PutUint16(buf[off:], 1)
	off += 2
	binary.BigEndian.PutUint64(buf[off:], b.ID)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], b.PrevHash)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], b.Hash)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(b.CreatedAt.UnixNano()))
	off += 8
	binary.BigEndian.PutUint16(buf[off:], uint16(len(b.State)))
	off += 2
	copy(buf[off:], []byte(b.State))
	off += len(b.State)
	binary.BigEndian.PutUint64(buf[off:], uint64(len(b.Records)))
	off += 8
	copy(buf[off:], records)
	return buf
}

// DecodeBlock 从二进制数据还原一个日志块。
func DecodeBlock(data []byte) (*Block, error) {
	if len(data) < 48 {
		return nil, fmt.Errorf("block payload too short: %d", len(data))
	}
	if binary.BigEndian.Uint32(data[0:4]) != blockMagic {
		return nil, errors.New("block magic mismatch")
	}
	b := &Block{}
	b.ID = binary.BigEndian.Uint64(data[6:14])
	b.PrevHash = binary.BigEndian.Uint64(data[14:22])
	b.Hash = binary.BigEndian.Uint64(data[22:30])
	b.CreatedAt = time.Unix(0, int64(binary.BigEndian.Uint64(data[30:38]))).UTC()
	stateLen := int(binary.BigEndian.Uint16(data[38:40]))
	if stateLen < 0 || stateLen > 16 || len(data) < 48+stateLen {
		return nil, errors.New("block state field invalid")
	}
	b.State = State(data[40 : 40+stateLen])
	recordCount := int(binary.BigEndian.Uint64(data[40+stateLen : 48+stateLen]))
	records, err := DecodeRecords(data[48+stateLen:], recordCount)
	if err != nil {
		return nil, err
	}
	b.Records = records
	return b, nil
}

// EncodeRecords 将记录数组编码为定长记录区。
func EncodeRecords(records []Record) []byte {
	total := 0
	for _, rec := range records {
		total += 4 + len(rec.Encode())
	}
	buf := make([]byte, total)
	off := 0
	for _, rec := range records {
		encoded := rec.Encode()
		binary.BigEndian.PutUint32(buf[off:], uint32(len(encoded)))
		off += 4
		copy(buf[off:], encoded)
		off += len(encoded)
	}
	return buf
}

// DecodeRecords 从记录区还原记录数组。
func DecodeRecords(data []byte, count int) ([]Record, error) {
	if count < 0 {
		return nil, errors.New("negative record count")
	}
	records := make([]Record, 0, count)
	off := 0
	for i := 0; i < count; i++ {
		if off+4 > len(data) {
			return nil, fmt.Errorf("block record %d truncated", i)
		}
		size := int(binary.BigEndian.Uint32(data[off:]))
		off += 4
		if off+size > len(data) {
			return nil, fmt.Errorf("block record %d exceeds payload", i)
		}
		rec, err := DecodeRecord(data[off : off+size])
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
		off += size
	}
	if off != len(data) {
		return nil, fmt.Errorf("block payload has %d trailing bytes", len(data)-off)
	}
	return records, nil
}

// RecordCount 返回块内记录条数。
func (b *Block) RecordCount() int {
	if b == nil {
		return 0
	}
	return len(b.Records)
}

// LastSeq 返回块内最后一条记录的序号；空块返回 0。
func (b *Block) LastSeq() uint64 {
	if b == nil || len(b.Records) == 0 {
		return 0
	}
	return b.Records[len(b.Records)-1].Seq
}

// Validate 校验块内部一致性：状态合法、记录序号连续递增。
func (b *Block) Validate() error {
	switch b.State {
	case StateActive, StateArchived, StateExpired:
	default:
		return fmt.Errorf("block %d has invalid state %q", b.ID, b.State)
	}
	expected := uint64(0)
	if len(b.Records) > 0 {
		expected = b.Records[0].Seq
	}
	for i, rec := range b.Records {
		if rec.Seq != expected {
			return fmt.Errorf("block %d record %d seq %d out of order", b.ID, i, rec.Seq)
		}
		expected++
	}
	return nil
}
