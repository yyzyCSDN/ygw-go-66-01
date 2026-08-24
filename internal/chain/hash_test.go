package chain

import (
	"testing"
	"time"

	"auditlog/internal/model"
)

func TestBlockHashDeterministic(t *testing.T) {
	block := &model.Block{
		ID:        1,
		CreatedAt: time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC),
		Records: []model.Record{
			{Seq: 1, WrittenAt: time.Date(2026, 8, 24, 8, 0, 1, 0, time.UTC), Actor: "a", Action: "x", Detail: "d"},
		},
	}
	first := BlockHash(block)
	second := BlockHash(block)
	if first != second {
		t.Fatalf("hash not deterministic: %d vs %d", first, second)
	}
	if first == 0 {
		t.Fatal("hash must not be zero")
	}
}

func TestBlockHashChangesWithContent(t *testing.T) {
	first := &model.Block{ID: 1, CreatedAt: time.Now(), Records: []model.Record{{Seq: 1, Actor: "a", Action: "x"}}}
	second := &model.Block{ID: 1, CreatedAt: time.Now(), Records: []model.Record{{Seq: 1, Actor: "b", Action: "x"}}}
	if BlockHash(first) == BlockHash(second) {
		t.Fatal("different content must produce different hashes")
	}
}

func TestDigestHex(t *testing.T) {
	text := DigestHex(0xabcd)
	if len(text) != 16 {
		t.Fatalf("digest length = %d", len(text))
	}
}
