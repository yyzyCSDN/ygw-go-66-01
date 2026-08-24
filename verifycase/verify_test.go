package verifycase

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"
	"auditlog/internal/chain"
	"auditlog/internal/model"
	"auditlog/internal/store"
)

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func newStore(t *testing.T) (*store.FileStore, func()) {
	t.Helper()
	st, err := store.NewFileStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return st, func() { _ = st.Close() }
}

// TestHashChainLinksPreviousBlock 校验新块必须链接上一块的哈希。
func TestHashChainLinksPreviousBlock(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	ch := chain.NewChain()
	now := time.Now().UTC()
	first := &model.Block{
		ID:        1,
		CreatedAt: now,
		State:     model.StateActive,
		Records:   []model.Record{{Seq: 1, WrittenAt: now, Actor: "ops", Action: "start", Detail: "boot"}},
	}
	second := &model.Block{
		ID:        2,
		CreatedAt: now.Add(time.Minute),
		State:     model.StateActive,
		Records:   []model.Record{{Seq: 2, WrittenAt: now.Add(time.Minute), Actor: "ops", Action: "stop", Detail: "halt"}},
	}
	if err := ch.Link(nil, first); err != nil {
		t.Fatalf("link first: %v", err)
	}
	if err := ch.Link(first, second); err != nil {
		t.Fatalf("link second: %v", err)
	}
	want := chain.BlockHash(first)
	if second.PrevHash != want {
		t.Fatalf("second.PrevHash = %d, want %d", second.PrevHash, want)
	}
	if second.Hash != chain.BlockHash(second) {
		t.Fatal("second self hash mismatch")
	}
	if err := ch.VerifyLink(first, second); err != nil {
		t.Fatalf("verify link: %v", err)
	}
	_ = st
}
