package append

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"auditlog/internal/chain"
	"auditlog/internal/model"
	"auditlog/internal/store"
)

// DefaultBlockSize 是未显式配置时的默认块容量。
const DefaultBlockSize = 256

// Metrics 汇总追加写路径的统计计数。
type Metrics struct {
	AppendTotal   uint64
	AppendErrors  uint64
	BlocksSealed  uint64
	RecordsTotal  uint64
	RotationsDone uint64
}

// Appender 负责日志块的创建、记录追加、链接与持久化。
type Appender struct {
	store     *store.FileStore
	chain     *chain.Chain
	blockSize int
	logger    *log.Logger

	mu          sync.Mutex
	head        *model.Block
	headFile    *os.File
	currentPath string
	seq         uint64
	metrics     Metrics
}

// NewAppender 创建追加器并从数据目录恢复头块与序号。
func NewAppender(st *store.FileStore, ch *chain.Chain, blockSize int, logger *log.Logger) (*Appender, error) {
	if st == nil {
		return nil, errors.New("append store is required")
	}
	if ch == nil {
		return nil, errors.New("append chain is required")
	}
	if blockSize <= 0 {
		blockSize = DefaultBlockSize
	}
	if logger == nil {
		logger = log.New(os.Stderr, "[append] ", log.LstdFlags)
	}
	appender := &Appender{
		store:     st,
		chain:     ch,
		blockSize: blockSize,
		logger:    logger,
	}
	if err := appender.recoverHead(); err != nil {
		return nil, err
	}
	return appender, nil
}

// recoverHead 从元数据与块文件恢复当前头块和序号。
func (a *Appender) recoverHead() error {
	paths := a.store.Paths()
	headBytes, err := a.store.ReadAll(paths.HeadFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read head file: %w", err)
	}
	if len(headBytes) != 8 {
		return fmt.Errorf("head file has invalid size %d", len(headBytes))
	}
	var id uint64
	for _, b := range headBytes {
		id = id<<8 | uint64(b)
	}
	block, err := a.ReadBlock(id)
	if err != nil {
		return fmt.Errorf("recover head block %d: %w", id, err)
	}
	a.head = block
	a.seq = block.LastSeq()
	a.chain.Register(id)
	file, err := a.store.OpenAppend(paths.BlockFile(id))
	if err != nil {
		return fmt.Errorf("open head block file: %w", err)
	}
	a.headFile = file
	a.currentPath = paths.BlockFile(id)
	return nil
}

// Append 写入一条审计记录：分配序号、追加到当前块、链接并持久化。
func (a *Appender) Append(rec model.Record) (model.Record, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if rec.WrittenAt.IsZero() {
		rec.WrittenAt = time.Now().UTC()
	}
	if a.seq == 0 {
		a.seq = 1
	}
	rec.Seq = a.seq
	if err := rec.Validate(); err != nil {
		return model.Record{}, err
	}
	a.seq++

	if a.head == nil {
		if err := a.startNewBlock(rec); err != nil {
			a.metrics.AppendErrors++
			return model.Record{}, err
		}
		a.metrics.AppendTotal++
		a.metrics.RecordsTotal++
		return rec, nil
	}
	if a.blockFull(a.head) {
		if err := a.startNewBlock(rec); err != nil {
			a.metrics.AppendErrors++
			return model.Record{}, err
		}
		a.metrics.AppendTotal++
		a.metrics.RecordsTotal++
		return rec, nil
	}
	if err := a.appendToBlock(a.head, rec); err != nil {
		a.metrics.AppendErrors++
		return model.Record{}, err
	}
	prevHash := a.head.Hash
	if err := a.chain.RefreshHash(a.head); err != nil {
		a.metrics.AppendErrors++
		return model.Record{}, err
	}
	if err := a.persistHead(); err != nil {
		// 持久化失败必须真实上报：吞错后仍返回成功会让接口对外宣称写入完成，
		// 而日志实际未落盘，造成审计记录静默缺失。回滚内存中已追加但未落盘的
		// 记录、哈希与序号，使调用方重试时可使用同一序号重新写入，不留空洞。
		a.logger.Printf("append persist failed: %v", err)
		a.head.Records = a.head.Records[:len(a.head.Records)-1]
		a.head.Hash = prevHash
		a.seq = rec.Seq
		a.metrics.AppendErrors++
		return model.Record{}, fmt.Errorf("persist head block %d: %w", a.head.ID, err)
	}
	a.metrics.AppendTotal++
	a.metrics.RecordsTotal++
	return rec, nil
}

// appendToBlock 将记录合并进目标块。
func (a *Appender) appendToBlock(block *model.Block, rec model.Record) error {
	if block == nil {
		return errors.New("append target block is nil")
	}
	last := block.LastSeq()
	if last != 0 && rec.Seq != last+1 {
		return fmt.Errorf("append seq %d does not continue block tail %d", rec.Seq, last)
	}
	block.Records = append(block.Records, rec)
	return nil
}

// blockFull 判断块容量是否已满。
func (a *Appender) blockFull(block *model.Block) bool {
	return block != nil && len(block.Records) >= a.blockSize
}

// startNewBlock 创建下一个块并链接到当前头块。
func (a *Appender) startNewBlock(rec model.Record) error {
	next := a.headID() + 1
	block := &model.Block{
		ID:        next,
		CreatedAt: rec.WrittenAt,
		Records:   []model.Record{rec},
		State:     model.StateActive,
	}
	if err := a.chain.LinkOrRollback(a.head, block); err != nil {
		return fmt.Errorf("link block %d: %w", block.ID, err)
	}
	path := a.store.Paths().BlockFile(block.ID)
	file, err := a.store.OpenAppend(path)
	if err != nil {
		return err
	}
	if a.headFile != nil {
		if err := a.store.CloseFile(a.currentPath); err != nil {
			return err
		}
	}
	block.File = path
	a.head = block
	a.headFile = file
	a.currentPath = path
	if err := a.writeBlock(block); err != nil {
		return err
	}
	if err := a.saveHeadID(block.ID); err != nil {
		return err
	}
	a.metrics.BlocksSealed++
	return nil
}

// persistHead 将头块写入文件并同步。
func (a *Appender) persistHead() error {
	if err := a.writeBlock(a.head); err != nil {
		return err
	}
	if err := a.store.SyncPath(a.currentPath); err != nil {
		return err
	}
	return a.saveHeadID(a.head.ID)
}

// writeBlock 把块编码后整体覆写头块文件。
func (a *Appender) writeBlock(block *model.Block) error {
	if block == nil || a.headFile == nil {
		return errors.New("head block is not ready")
	}
	data := block.Encode()
	if err := a.headFile.Truncate(0); err != nil {
		return fmt.Errorf("truncate block file: %w", err)
	}
	if _, err := a.headFile.WriteAt(data, 0); err != nil {
		return fmt.Errorf("write block file: %w", err)
	}
	if err := a.headFile.Sync(); err != nil {
		return fmt.Errorf("sync block file: %w", err)
	}
	return nil
}

// saveHeadID 将头块序号写入元数据文件。
func (a *Appender) saveHeadID(id uint64) error {
	var data [8]byte
	for i := 7; i >= 0; i-- {
		data[i] = byte(id & 0xff)
		id >>= 8
	}
	return a.store.WriteFile(a.store.Paths().HeadFile(), data[:])
}

// headID 返回当前头块序号，无头块时返回 0。
func (a *Appender) headID() uint64 {
	if a.head == nil {
		return 0
	}
	return a.head.ID
}

// CurrentBlockID 返回当前头块序号。
func (a *Appender) CurrentBlockID() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.headID()
}

// NextSeq 返回下一条记录的预分配序号。
func (a *Appender) NextSeq() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seq == 0 {
		return 1
	}
	return a.seq + 1
}

// SnapshotMetrics 返回追加器统计的当前快照。
func (a *Appender) SnapshotMetrics() Metrics {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.metrics
}
