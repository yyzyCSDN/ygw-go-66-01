package verifycase

import (
	"errors"
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

// TestMissingBlockNoNilPanic 校验缺失块返回错误而非空引用。
func TestMissingBlockNoNilPanic(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: "one"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on missing block: %v", r)
		}
	}()
	block, err := app.ReadBlock(999)
	if err == nil {
		_ = block.Records
		t.Fatal("missing block must return an error")
	}
	if !errors.Is(err, appender.ErrBlockNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}
