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
	"auditlog/internal/verify"
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

// TestVerifyFailureNotSwallowed 校验篡改后的链必须报失败。
func TestVerifyFailureNotSwallowed(t *testing.T) {
	st, cleanup := newStore(t)
	defer cleanup()
	app, err := appender.NewAppender(st, chain.NewChain(), 1, discardLogger())
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if _, err := app.Append(model.Record{Actor: "svc", Action: "write", Detail: fmt.Sprintf("r%d", i)}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	path := st.Paths().BlockFile(2)
	data, err := st.ReadAll(path)
	if err != nil {
		t.Fatalf("read block file: %v", err)
	}
	block, err := model.DecodeBlock(data)
	if err != nil {
		t.Fatalf("decode block: %v", err)
	}
	block.Records[0].Detail = "tampered"
	if err := st.WriteFile(path, block.Encode()); err != nil {
		t.Fatalf("rewrite block: %v", err)
	}
	verifier, err := verify.NewVerifier(chain.NewChain(), app, discardLogger())
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	report, err := verifier.VerifyAll()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.Passed {
		t.Fatal("tampered chain must fail verification")
	}
	if len(report.Broken) == 0 {
		t.Fatal("broken links must be reported")
	}
}
