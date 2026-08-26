package verifycase

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"testing"
	appender "auditlog/internal/append"
	"auditlog/internal/chain"
	"auditlog/internal/export"
	"auditlog/internal/index"
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

// TestExportCursorUsesLatestIndex 校验续传按最新索引推进。
func TestExportCursorUsesLatestIndex(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	idx := index.NewIndex()
	ex := export.NewExporter(st, idx, app, discardLogger())
	for i := 1; i <= 3; i++ {
		rec, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: fmt.Sprintf("a%d", i)})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		_ = idx.Add(rec)
	}
	cursor, err := ex.Start()
	if err != nil {
		t.Fatalf("start export: %v", err)
	}
	for i := 4; i <= 6; i++ {
		rec, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: fmt.Sprintf("b%d", i)})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		_ = idx.Add(rec)
	}
	resumed, err := ex.Resume(cursor.ExporterID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	records, err := ex.Next(resumed, 100)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if len(records) != 6 {
		t.Fatalf("exported %d records, want 6", len(records))
	}
	if !resumed.Complete {
		t.Fatalf("cursor not complete: %+v", resumed)
	}
}
