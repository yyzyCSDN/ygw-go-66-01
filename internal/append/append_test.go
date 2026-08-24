package append

import (
	"log"
	"path/filepath"
	"testing"

	"auditlog/internal/chain"
	"auditlog/internal/store"
)

func TestNewAppenderEmptyDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	fileStore, err := store.NewFileStore(root)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer fileStore.Close()
	appender, err := NewAppender(fileStore, chain.NewChain(), 0, log.New(testWriter{}, "", 0))
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	if appender.CurrentBlockID() != 0 {
		t.Fatalf("head = %d", appender.CurrentBlockID())
	}
	if appender.NextSeq() != 1 {
		t.Fatalf("next seq = %d", appender.NextSeq())
	}
	metrics := appender.SnapshotMetrics()
	if metrics.AppendTotal != 0 || metrics.BlocksSealed != 0 {
		t.Fatalf("metrics = %+v", metrics)
	}
}

type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
