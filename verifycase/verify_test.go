package verifycase

import (
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

// TestAppendErrorNotSwallowed 校验追加失败必须上报。
func TestAppendErrorNotSwallowed(t *testing.T) {
	st, _ := newStore(t)
	app, err := appender.NewAppender(st, chain.NewChain(), 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: "one"}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: "two"}); err == nil {
		t.Fatal("append after store close must return an error")
	}
}
