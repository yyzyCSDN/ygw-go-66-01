package model

import (
	"encoding/binary"
	"fmt"
	"time"
)

// JournalEntry 是块元数据日志中的一条记录，描述块在某时刻发生的状态动作。
type JournalEntry struct {
	BlockID uint64
	Action  string
	At      time.Time
	Meta    string
}

// Encode 序列化日志条目。
func (j JournalEntry) Encode() []byte {
	action := []byte(j.Action)
	meta := []byte(j.Meta)
	buf := make([]byte, 8+8+4+len(action)+4+len(meta))
	off := 0
	binary.BigEndian.PutUint64(buf[off:], j.BlockID)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(j.At.UnixNano()))
	off += 8
	binary.BigEndian.PutUint32(buf[off:], uint32(len(action)))
	off += 4
	copy(buf[off:], action)
	off += len(action)
	binary.BigEndian.PutUint32(buf[off:], uint32(len(meta)))
	off += 4
	copy(buf[off:], meta)
	return buf
}

// DecodeJournalEntry 还原日志条目。
func DecodeJournalEntry(data []byte) (JournalEntry, error) {
	if len(data) < 16 {
		return JournalEntry{}, fmt.Errorf("journal entry too short: %d", len(data))
	}
	entry := JournalEntry{}
	entry.BlockID = binary.BigEndian.Uint64(data[0:8])
	entry.At = time.Unix(0, int64(binary.BigEndian.Uint64(data[8:16]))).UTC()
	off := 16
	actionLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+actionLen > len(data) {
		return JournalEntry{}, fmt.Errorf("journal action field truncated")
	}
	entry.Action = string(data[off : off+actionLen])
	off += actionLen
	if off+4 > len(data) {
		return JournalEntry{}, fmt.Errorf("journal meta length truncated")
	}
	metaLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+metaLen > len(data) {
		return JournalEntry{}, fmt.Errorf("journal meta field truncated")
	}
	entry.Meta = string(data[off : off+metaLen])
	return entry, nil
}
