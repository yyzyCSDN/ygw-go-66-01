package verifycase

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	appender "auditlog/internal/append"
	"auditlog/internal/archive"
	"auditlog/internal/chain"
	"auditlog/internal/index"
	"auditlog/internal/model"
	"auditlog/internal/reten"
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

// TestRetentionKeepsFreshLogs 校验未到期日志不被清理。
func TestRetentionKeepsFreshLogs(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	ch := chain.NewChain()
	app, err := appender.NewAppender(st, ch, 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	idx := index.NewIndex()
	ar, err := archive.NewArchiver(st, discardLogger())
	if err != nil {
		t.Fatalf("archiver: %v", err)
	}
	rt := reten.NewRetention(st, ar, app, discardLogger())
	rec, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: "fresh"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	_ = idx.Add(rec)
	policy, err := reten.ParsePolicy("7d")
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	report, err := rt.Apply(policy)
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if report.Deleted != 0 {
		t.Fatalf("fresh block deleted: %d", report.Deleted)
	}
	if _, err := app.ReadBlock(1); err != nil {
		t.Fatalf("fresh block missing: %v", err)
	}
}
