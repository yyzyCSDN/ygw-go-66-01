package archive

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"auditlog/internal/chain"
	"auditlog/internal/model"
	"auditlog/internal/store"
)

// Archiver 按窗口把活跃日志块迁移到归档区并维护归档清单。
type Archiver struct {
	store       *store.FileStore
	logger      *log.Logger
	mu          sync.Mutex
	manifest    *Manifest
	windowStart uint64
	windowEnd   uint64
}

// NewArchiver 创建归档器并从磁盘恢复清单。
func NewArchiver(st *store.FileStore, logger *log.Logger) (*Archiver, error) {
	if st == nil {
		return nil, errors.New("archive store is required")
	}
	if logger == nil {
		logger = log.New(os.Stderr, "[archive] ", log.LstdFlags)
	}
	archiver := &Archiver{store: st, logger: logger, manifest: NewManifest()}
	if err := archiver.loadManifest(); err != nil {
		return nil, err
	}
	return archiver, nil
}

// Window 从活跃块中选择一个归档窗口：从最旧活跃块开始的 size 个连续活跃块。
// 选中后同步记录窗口边界，供保留策略跳过尚未落定归档的边界块，避免误清理。
func (ar *Archiver) Window(blocks []*model.Block, size int) ([]*model.Block, error) {
	if size <= 0 {
		return nil, errors.New("archive window size must be positive")
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	sorted := make([]*model.Block, len(blocks))
	copy(sorted, blocks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	// 窗口从最旧的活跃块起算，跳过已归档块，保证连续调用能逐批推进。
	selected := make([]*model.Block, 0, size)
	for _, block := range sorted {
		if len(selected) >= size {
			break
		}
		if block.State == model.StateActive {
			selected = append(selected, block)
		}
	}
	// 边界采用半开区间 [start, end)，与 InActiveWindow 的判定一致。
	var start, end uint64
	if len(selected) > 0 {
		start = selected[0].ID
		end = selected[len(selected)-1].ID + 1
	}
	ar.mu.Lock()
	ar.windowStart = start
	ar.windowEnd = end
	ar.mu.Unlock()
	return selected, nil
}


// InActiveWindow 判断块序号是否落在最近一次归档窗口内。保留策略用它
// 跳过尚未落定归档的边界块，避免窗口内的块被误清理。
func (ar *Archiver) InActiveWindow(id uint64) bool {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	return ar.windowStart != 0 && id >= ar.windowStart && id < ar.windowEnd
}

// Archive 把选定块迁移到归档区并更新清单。
func (ar *Archiver) Archive(blocks []*model.Block) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	paths := ar.store.Paths()
	for _, block := range blocks {
		if ar.manifest.Has(block.ID) {
			ar.logger.Printf("block %d already archived; skipped", block.ID)
			continue
		}
		if err := chain.TransitionAllowed(block.State, model.StateArchived); err != nil {
			return fmt.Errorf("archive block %d: %w", block.ID, err)
		}
		source := paths.BlockFile(block.ID)
		destination := paths.ArchiveBlockFile(block.ID)
		data, err := ar.store.ReadAll(source)
		if err != nil {
			return fmt.Errorf("read block %d for archive: %w", block.ID, err)
		}
		if err := ar.store.WriteFile(destination, data); err != nil {
			return fmt.Errorf("write archived block %d: %w", block.ID, err)
		}
		if err := ar.store.Remove(source); err != nil {
			return fmt.Errorf("remove active block %d: %w", block.ID, err)
		}
		block.State = chain.NextState(block.State)
		block.File = destination
		ar.manifest.Add(block.ID, time.Now().UTC())
		ar.logger.Printf("archived block %d (%d records, state %s)",
			block.ID, block.RecordCount(), chain.StateLabel(block.State))
	}
	return ar.saveManifest()
}

// Restore 把指定块从归档区恢复到活跃区。
func (ar *Archiver) Restore(id uint64) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	paths := ar.store.Paths()
	source := paths.ArchiveBlockFile(id)
	if !ar.store.Exists(source) {
		return fmt.Errorf("archived block %d not found", id)
	}
	data, err := ar.store.ReadAll(source)
	if err != nil {
		return err
	}
	destination := paths.BlockFile(id)
	if err := ar.store.WriteFile(destination, data); err != nil {
		return err
	}
	if err := ar.store.Remove(source); err != nil {
		return err
	}
	ar.manifest.Remove(id)
	return ar.saveManifest()
}

// ArchivedSince 返回块的归档时间，供保留策略跳过已归档块。
func (ar *Archiver) ArchivedSince(id uint64) (time.Time, bool) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	return ar.manifest.ArchivedSince(id)
}

// ManifestSnapshot 返回归档清单的副本。
func (ar *Archiver) ManifestSnapshot() *Manifest {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	copyManifest := NewManifest()
	copyManifest.Version = ar.manifest.Version
	copyManifest.UpdatedAt = ar.manifest.UpdatedAt
	for id, at := range ar.manifest.Archived {
		copyManifest.Archived[id] = at
	}
	return copyManifest
}

func (ar *Archiver) manifestPath() string {
	return ar.store.Paths().MetaDir() + string(os.PathSeparator) + "archive-manifest.bin"
}

func (ar *Archiver) loadManifest() error {
	path := ar.manifestPath()
	if !ar.store.Exists(path) {
		return nil
	}
	data, err := ar.store.ReadAll(path)
	if err != nil {
		return fmt.Errorf("read archive manifest: %w", err)
	}
	manifest, err := DecodeManifest(data)
	if err != nil {
		return fmt.Errorf("decode archive manifest: %w", err)
	}
	ar.manifest = manifest
	return nil
}

func (ar *Archiver) saveManifest() error {
	path := ar.manifestPath()
	if err := ar.store.WriteFile(path, ar.manifest.Encode()); err != nil {
		return fmt.Errorf("save archive manifest: %w", err)
	}
	return nil
}
