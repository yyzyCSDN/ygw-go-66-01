package verifycase

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"testing"
	appender "auditlog/internal/append"
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

// TestAppendOffsetNoOverwrite 校验追加不覆盖已有记录。
func TestAppendOffsetNoOverwrite(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: fmt.Sprintf("entry-%d", i)}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	block, err := app.ReadBlock(1)
	if err != nil {
		t.Fatalf("read block: %v", err)
	}
	if block.RecordCount() != 3 {
		t.Fatalf("records = %d, want 3", block.RecordCount())
	}
	for i, rec := range block.Records {
		if rec.Seq != uint64(i+1) {
			t.Fatalf("record %d has seq %d", i, rec.Seq)
		}
	}
}
