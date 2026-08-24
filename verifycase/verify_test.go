package verifycase

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"testing"
	appender "auditlog/internal/append"
	"auditlog/internal/chain"
	"auditlog/internal/index"
	"auditlog/internal/model"
	"auditlog/internal/search"
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

// TestSearchSliceKeepsAllRecords 校验分页不丢记录。
func TestSearchSliceKeepsAllRecords(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 64, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	idx := index.NewIndex()
	for i := 1; i <= 10; i++ {
		rec, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: fmt.Sprintf("payload-%d", i)})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if err := idx.Add(rec); err != nil {
			t.Fatalf("index %d: %v", i, err)
		}
	}
	searcher, err := search.NewSearcher(idx, app)
	if err != nil {
		t.Fatalf("searcher: %v", err)
	}
	total := 0
	for page := 1; page <= 3; page++ {
		result, err := searcher.Page(index.Query{Actor: "svc"}, page, 4)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		total += len(result.Records)
	}
	if total != 10 {
		t.Fatalf("paged total = %d, want 10", total)
	}
}
