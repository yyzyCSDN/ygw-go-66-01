package verifycase

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"
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

// TestLogFileHandleClosed 校验轮转后旧句柄被关闭。
func TestLogFileHandleClosed(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: "one"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := app.Rotate(now.Add(time.Duration(i+1) * 24 * time.Hour)); err != nil {
			t.Fatalf("rotate %d: %v", i, err)
		}
	}
	if count := st.OpenFileCount(); count > 2 {
		t.Fatalf("open file handles = %d, want <= 2", count)
	}
}
