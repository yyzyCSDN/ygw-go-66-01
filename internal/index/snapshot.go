package index

import (
	"encoding/binary"
	"fmt"
	"sort"
	"time"
)

// TimedSeq 是时间线上的一条记录位置。
type TimedSeq struct {
	At  time.Time
	Seq uint64
}

// Entry 是快照中的一个倒排桶。
type Entry struct {
	Kind string
	Key  string
	Seqs []uint64
}

// Snapshot 是索引在某代次的完整视图，可持久化后用于重启恢复。
type Snapshot struct {
	Generation uint64
	Total      int
	Entries    []Entry
	Timeline   []TimedSeq
}

// Snapshot 返回当前索引的完整快照。
func (x *Index) Snapshot() *Snapshot {
	x.mu.RLock()
	defer x.mu.RUnlock()
	snapshot := &Snapshot{
		Generation: x.generation,
		Total:      x.total,
		Timeline:   make([]TimedSeq, len(x.timeline)),
	}
	copy(snapshot.Timeline, x.timeline)
	keys := make([]string, 0, len(x.actors)+len(x.actions)+len(x.keywords))
	byKind := map[string]map[string]*Bucket{
		"actor":   x.actors,
		"action":  x.actions,
		"keyword": x.keywords,
	}
	for kind, buckets := range byKind {
		for key, bucket := range buckets {
			if bucket == nil {
				continue
			}
			keys = append(keys, key)
			entry := Entry{
				Kind: kind,
				Key:  key,
				Seqs: make([]uint64, len(bucket.Seqs)),
			}
			copy(entry.Seqs, bucket.Seqs)
			snapshot.Entries = append(snapshot.Entries, entry)
		}
	}
	sort.Slice(snapshot.Entries, func(i, j int) bool {
		if snapshot.Entries[i].Kind != snapshot.Entries[j].Kind {
			return snapshot.Entries[i].Kind < snapshot.Entries[j].Kind
		}
		return snapshot.Entries[i].Key < snapshot.Entries[j].Key
	})
	return snapshot
}

// Restore 用快照覆盖当前索引内容。
func (x *Index) Restore(snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("cannot restore a nil snapshot")
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	rebuilt := NewIndex()
	for _, entry := range snapshot.Entries {
		bucket := &Bucket{Key: entry.Key, Seqs: append([]uint64(nil), entry.Seqs...)}
		switch entry.Kind {
		case "actor":
			rebuilt.actors[entry.Key] = bucket
		case "action":
			rebuilt.actions[entry.Key] = bucket
		case "keyword":
			rebuilt.keywords[entry.Key] = bucket
		default:
			return fmt.Errorf("unknown snapshot entry kind %q", entry.Kind)
		}
	}
	rebuilt.timeline = append([]TimedSeq(nil), snapshot.Timeline...)
	rebuilt.total = snapshot.Total
	rebuilt.generation = snapshot.Generation
	x.actors = rebuilt.actors
	x.actions = rebuilt.actions
	x.keywords = rebuilt.keywords
	x.timeline = rebuilt.timeline
	x.total = rebuilt.total
	x.generation = rebuilt.generation
	return nil
}

// EncodeSnapshot 序列化索引快照。
func EncodeSnapshot(snapshot *Snapshot) []byte {
	var total int
	for _, entry := range snapshot.Entries {
		total += 8 + 4 + len(entry.Kind) + 4 + len(entry.Key) + 4 + 8*len(entry.Seqs)
	}
	total += 8 + 8 + 8 + 8*len(snapshot.Timeline) + 8
	buf := make([]byte, total)
	off := 0
	binary.BigEndian.PutUint64(buf[off:], snapshot.Generation)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(snapshot.Total))
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(len(snapshot.Entries)))
	off += 8
	for _, entry := range snapshot.Entries {
		binary.BigEndian.PutUint32(buf[off:], uint32(len(entry.Kind)))
		off += 4
		copy(buf[off:], []byte(entry.Kind))
		off += len(entry.Kind)
		binary.BigEndian.PutUint32(buf[off:], uint32(len(entry.Key)))
		off += 4
		copy(buf[off:], []byte(entry.Key))
		off += len(entry.Key)
		binary.BigEndian.PutUint64(buf[off:], uint64(len(entry.Seqs)))
		off += 8
		for _, seq := range entry.Seqs {
			binary.BigEndian.PutUint64(buf[off:], seq)
			off += 8
		}
	}
	binary.BigEndian.PutUint64(buf[off:], uint64(len(snapshot.Timeline)))
	off += 8
	for _, item := range snapshot.Timeline {
		binary.BigEndian.PutUint64(buf[off:], uint64(item.At.UnixNano()))
		off += 8
		binary.BigEndian.PutUint64(buf[off:], item.Seq)
		off += 8
	}
	return buf
}

// DecodeSnapshot 还原索引快照。
func DecodeSnapshot(data []byte) (*Snapshot, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("snapshot payload too short: %d", len(data))
	}
	snapshot := &Snapshot{}
	snapshot.Generation = binary.BigEndian.Uint64(data[0:8])
	snapshot.Total = int(binary.BigEndian.Uint64(data[8:16]))
	entryCount := int(binary.BigEndian.Uint64(data[16:24]))
	off := 24
	for i := 0; i < entryCount; i++ {
		kind, err := readStringField(data, &off)
		if err != nil {
			return nil, err
		}
		key, err := readStringField(data, &off)
		if err != nil {
			return nil, err
		}
		if off+8 > len(data) {
			return nil, fmt.Errorf("snapshot entry %d seq count truncated", i)
		}
		seqCount := int(binary.BigEndian.Uint64(data[off:]))
		off += 8
		if off+8*seqCount > len(data) {
			return nil, fmt.Errorf("snapshot entry %d seqs exceed payload", i)
		}
		seqs := make([]uint64, 0, seqCount)
		for j := 0; j < seqCount; j++ {
			seqs = append(seqs, binary.BigEndian.Uint64(data[off:]))
			off += 8
		}
		snapshot.Entries = append(snapshot.Entries, Entry{Kind: kind, Key: key, Seqs: seqs})
	}
	if off+8 > len(data) {
		return nil, fmt.Errorf("snapshot timeline count truncated")
	}
	timelineCount := int(binary.BigEndian.Uint64(data[off:]))
	off += 8
	for i := 0; i < timelineCount; i++ {
		if off+16 > len(data) {
			return nil, fmt.Errorf("snapshot timeline entry %d truncated", i)
		}
		at := time.Unix(0, int64(binary.BigEndian.Uint64(data[off:]))).UTC()
		seq := binary.BigEndian.Uint64(data[off+8:])
		off += 16
		snapshot.Timeline = append(snapshot.Timeline, TimedSeq{At: at, Seq: seq})
	}
	if off != len(data) {
		return nil, fmt.Errorf("snapshot payload has %d trailing bytes", len(data)-off)
	}
	return snapshot, nil
}

func readStringField(data []byte, off *int) (string, error) {
	if *off+4 > len(data) {
		return "", fmt.Errorf("snapshot string length truncated at %d", *off)
	}
	size := int(binary.BigEndian.Uint32(data[*off:]))
	*off += 4
	if *off+size > len(data) {
		return "", fmt.Errorf("snapshot string field truncated at %d", *off)
	}
	value := string(data[*off : *off+size])
	*off += size
	return value, nil
}
