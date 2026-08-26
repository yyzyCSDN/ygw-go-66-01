package model

import (
	"encoding/binary"
	"fmt"
	"time"
)

// ExportCursor 记录一次导出任务的进度。LastSeq 是已导出记录的最大序号，
// BlockID 是最后处理的块，IndexSeq 是导出开始时索引快照的代次。
type ExportCursor struct {
	ExporterID string
	LastSeq    uint64
	BlockID    uint64
	IndexSeq   uint64
	UpdatedAt  time.Time
	Complete   bool
}

// Encode 序列化导出游标。
func (c ExportCursor) Encode() []byte {
	id := []byte(c.ExporterID)
	buf := make([]byte, 8+8+8+8+4+len(id)+1)
	off := 0
	binary.BigEndian.PutUint64(buf[off:], c.LastSeq)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], c.BlockID)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], c.IndexSeq)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(c.UpdatedAt.UnixNano()))
	off += 8
	binary.BigEndian.PutUint32(buf[off:], uint32(len(id)))
	off += 4
	copy(buf[off:], id)
	off += len(id)
	if c.Complete {
		buf[off] = 1
	}
	return buf
}

// DecodeExportCursor 还原导出游标。
func DecodeExportCursor(data []byte) (ExportCursor, error) {
	if len(data) < 37 {
		return ExportCursor{}, fmt.Errorf("export cursor too short: %d", len(data))
	}
	cur := ExportCursor{}
	cur.LastSeq = binary.BigEndian.Uint64(data[0:8])
	cur.BlockID = binary.BigEndian.Uint64(data[8:16])
	cur.IndexSeq = binary.BigEndian.Uint64(data[16:24])
	cur.UpdatedAt = time.Unix(0, int64(binary.BigEndian.Uint64(data[24:32]))).UTC()
	idLen := int(binary.BigEndian.Uint32(data[32:36]))
	if 36+idLen+1 > len(data) {
		return ExportCursor{}, fmt.Errorf("export cursor id field truncated")
	}
	cur.ExporterID = string(data[36 : 36+idLen])
	cur.Complete = data[36+idLen] == 1
	return cur, nil
}
