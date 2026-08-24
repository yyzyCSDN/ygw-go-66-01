package archive

import (
	"encoding/binary"
	"fmt"
	"time"
)

// Manifest 记录已归档块及其归档时间，用于恢复与保留策略联动。
type Manifest struct {
	Version   int
	UpdatedAt time.Time
	Archived  map[uint64]time.Time
}

// NewManifest 创建空归档清单。
func NewManifest() *Manifest {
	return &Manifest{Version: 1, Archived: make(map[uint64]time.Time)}
}

// Add 登记一个已归档块。
func (m *Manifest) Add(id uint64, at time.Time) {
	if m.Archived == nil {
		m.Archived = make(map[uint64]time.Time)
	}
	m.Archived[id] = at
	m.UpdatedAt = at
}

// Has 判断块是否已在归档清单中。
func (m *Manifest) Has(id uint64) bool {
	if m == nil || m.Archived == nil {
		return false
	}
	_, ok := m.Archived[id]
	return ok
}

// ArchivedSince 返回块的归档时间；未归档返回 false。
func (m *Manifest) ArchivedSince(id uint64) (time.Time, bool) {
	if m == nil || m.Archived == nil {
		return time.Time{}, false
	}
	at, ok := m.Archived[id]
	return at, ok
}

// Remove 从清单移除一个块。
func (m *Manifest) Remove(id uint64) {
	delete(m.Archived, id)
}

// Count 返回已归档块数量。
func (m *Manifest) Count() int {
	if m == nil || m.Archived == nil {
		return 0
	}
	return len(m.Archived)
}

// Encode 序列化归档清单。
func (m *Manifest) Encode() []byte {
	buf := make([]byte, 8+8+8+16*len(m.Archived))
	off := 0
	binary.BigEndian.PutUint64(buf[off:], uint64(m.Version))
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(m.UpdatedAt.UnixNano()))
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(len(m.Archived)))
	off += 8
	for id, at := range m.Archived {
		binary.BigEndian.PutUint64(buf[off:], id)
		off += 8
		binary.BigEndian.PutUint64(buf[off:], uint64(at.UnixNano()))
		off += 8
	}
	return buf
}

// DecodeManifest 还原归档清单。
func DecodeManifest(data []byte) (*Manifest, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("archive manifest too short: %d", len(data))
	}
	manifest := NewManifest()
	manifest.Version = int(binary.BigEndian.Uint64(data[0:8]))
	manifest.UpdatedAt = time.Unix(0, int64(binary.BigEndian.Uint64(data[8:16]))).UTC()
	count := int(binary.BigEndian.Uint64(data[16:24]))
	if 24+16*count > len(data) {
		return nil, fmt.Errorf("archive manifest entry count exceeds payload")
	}
	off := 24
	for i := 0; i < count; i++ {
		id := binary.BigEndian.Uint64(data[off:])
		at := time.Unix(0, int64(binary.BigEndian.Uint64(data[off+8:]))).UTC()
		off += 16
		manifest.Archived[id] = at
	}
	if off != len(data) {
		return nil, fmt.Errorf("archive manifest has %d trailing bytes", len(data)-off)
	}
	return manifest, nil
}
