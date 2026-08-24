package append

import (
	"log"
	"path/filepath"
	"testing"
	"time"

	"auditlog/internal/chain"
	"auditlog/internal/model"
	"auditlog/internal/store"
)

// validRecord 构造一条合法记录，序号由调用方指定。
func validRecord(seq uint64) model.Record {
	return model.Record{
		Seq:       seq,
		Actor:     "auditor",
		Action:    "create",
		Detail:    "test record",
		WrittenAt: time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
}

// TestAppendPersistFailureReportsError 验证持久化失败时接口不会返回成功：
// 日志实际未落盘时必须真实上报错误，否则审计记录会被静默丢失。
func TestAppendPersistFailureReportsError(t *testing.T) {
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

	// 先写入一条记录建立头块，保证后续走 appendToBlock + persistHead 路径。
	if _, err := appender.Append(validRecord(1)); err != nil {
		t.Fatalf("seed first record: %v", err)
	}

	// 关闭头块文件句柄，使 persistHead 内部 writeBlock 落盘失败。
	appender.mu.Lock()
	head := appender.head
	if head == nil || appender.headFile == nil {
		t.Fatalf("head block/file not ready after seed")
	}
	prevHash := head.Hash
	seqBefore := appender.seq // 失败前的序号基线，用于校验回滚
	if err := appender.headFile.Close(); err != nil {
		t.Fatalf("close head file: %v", err)
	}
	appender.mu.Unlock()

	_, err = appender.Append(validRecord(2))
	if err == nil {
		t.Fatalf("expected error when persist fails, got success (audit record would be lost)")
	}

	// 持久化失败必须计入错误指标，而非被吞掉后计入成功指标。
	metrics := appender.SnapshotMetrics()
	if metrics.AppendErrors == 0 {
		t.Fatalf("expected AppendErrors > 0 after persist failure, got %+v", metrics)
	}

	appender.mu.Lock()
	// 内存中已追加但未落盘的记录必须回滚，块记录数恢复为 1。
	if len(head.Records) != 1 {
		t.Fatalf("head records = %d after failed persist, want 1 (rolled back)", len(head.Records))
	}
	// 哈希必须回滚到持久化前的值，保持链状态一致。
	if head.Hash != prevHash {
		t.Fatalf("head hash changed after failed persist: got %d, want %d", head.Hash, prevHash)
	}
	// 序号必须回滚到失败前的基线，使调用方可用同一序号重试，不留空洞。
	if appender.seq != seqBefore {
		t.Fatalf("after failed append, seq = %d, want %d (rolled back for retry)", appender.seq, seqBefore)
	}
	appender.mu.Unlock()
}

// TestAppendRetryAfterPersistFailure 验证持久化失败并回滚后，使用同一序号重试
// 能成功落盘：审计记录最终不丢失，且序号连续。
func TestAppendRetryAfterPersistFailure(t *testing.T) {
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

	if _, err := appender.Append(validRecord(1)); err != nil {
		t.Fatalf("seed first record: %v", err)
	}

	// 触发一次持久化失败。
	appender.mu.Lock()
	_ = appender.headFile.Close()
	appender.mu.Unlock()
	if _, err := appender.Append(validRecord(2)); err == nil {
		t.Fatalf("expected error on first persist failure")
	}

	// 重新打开头块文件句柄，模拟底层故障恢复后重试。
	appender.mu.Lock()
	file, openErr := appender.store.OpenAppend(appender.currentPath)
	if openErr != nil {
		t.Fatalf("reopen head file: %v", openErr)
	}
	appender.headFile = file
	appender.mu.Unlock()

	// 用同一序号重试，应成功落盘。
	written, err := appender.Append(validRecord(2))
	if err != nil {
		t.Fatalf("retry append: %v", err)
	}
	if written.Seq != 2 {
		t.Fatalf("retry written seq = %d, want 2", written.Seq)
	}

	// 读回块文件确认重试的记录确实落盘，审计记录未丢失。
	block, err := appender.ReadBlock(appender.CurrentBlockID())
	if err != nil {
		t.Fatalf("read head block: %v", err)
	}
	if len(block.Records) != 2 {
		t.Fatalf("head block records = %d, want 2", len(block.Records))
	}
	if block.Records[1].Seq != 2 {
		t.Fatalf("second record seq = %d, want 2", block.Records[1].Seq)
	}
}
