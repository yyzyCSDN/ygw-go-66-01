package verifycase

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"testing"
	appender "auditlog/internal/append"
	"auditlog/internal/archive"
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

// TestArchiveWindowKeepsBoundaryBlocks 校验归档窗口边界块不误归档。
func TestArchiveWindowKeepsBoundaryBlocks(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 1, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	for i := 1; i <= 6; i++ {
		if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: fmt.Sprintf("r%d", i)}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	ar, err := archive.NewArchiver(st, discardLogger())
	if err != nil {
		t.Fatalf("archiver: %v", err)
	}
	blocks := make([]*model.Block, 0, 6)
	for id := uint64(1); id <= 6; id++ {
		block, err := app.ReadBlock(id)
		if err != nil {
			t.Fatalf("read block %d: %v", id, err)
		}
		blocks = append(blocks, block)
	}
	selected, err := ar.Window(blocks, 3)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	if len(selected) != 3 {
		t.Fatalf("window selected %d blocks, want 3", len(selected))
	}
	if err := ar.Archive(selected); err != nil {
		t.Fatalf("archive: %v", err)
	}
	manifest := ar.ManifestSnapshot()
	for id := uint64(1); id <= 3; id++ {
		if !manifest.Has(id) {
			t.Fatalf("block %d was not archived", id)
		}
	}
	if manifest.Has(4) {
		t.Fatal("block 4 must stay active")
	}
	if _, err := app.ReadBlock(4); err != nil {
		t.Fatalf("block 4 unreadable: %v", err)
	}
}
