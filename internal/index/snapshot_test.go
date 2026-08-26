package index

import (
	"reflect"
	"testing"
	"time"

	"auditlog/internal/model"
)

func TestSnapshotRoundTrip(t *testing.T) {
	idx := NewIndex()
	at := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		rec := model.Record{
			Seq:       uint64(i),
			WrittenAt: at.Add(time.Duration(i) * time.Minute),
			Actor:     "ops",
			Action:    "rotate",
			Detail:    "disk usr audit volume",
		}
		if err := idx.Add(rec); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	snapshot := idx.Snapshot()
	decoded, err := DecodeSnapshot(EncodeSnapshot(snapshot))
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if decoded.Generation != 3 || decoded.Total != 3 {
		t.Fatalf("snapshot meta = %+v", decoded)
	}
	restored := NewIndex()
	if err := restored.Restore(decoded); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Total() != 3 || restored.Generation() != 3 {
		t.Fatalf("restored index meta mismatch")
	}
	hits, total, err := restored.Query(Query{Actor: "ops"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 3 || !reflect.DeepEqual(hits, []uint64{1, 2, 3}) {
		t.Fatalf("query hits = %v total = %d", hits, total)
	}
}
