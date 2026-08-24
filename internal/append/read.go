package append

import (
	"errors"
	"fmt"
	"sort"

	"auditlog/internal/model"
	"auditlog/internal/store"
)

// ErrBlockNotFound 表示请求的块不存在。
var ErrBlockNotFound = errors.New("block not found")

// ReadBlock 按序号读取日志块，先查活跃区再查归档区。
func (a *Appender) ReadBlock(id uint64) (*model.Block, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	paths := a.store.Paths()
	for _, candidate := range []string{paths.BlockFile(id), paths.ArchiveBlockFile(id)} {
		if !a.store.Exists(candidate) {
			continue
		}
		data, err := a.store.ReadAll(candidate)
		if err != nil {
			return nil, fmt.Errorf("read block %d: %w", id, err)
		}
		block, err := model.DecodeBlock(data)
		if err != nil {
			return nil, fmt.Errorf("decode block %d: %w", id, err)
		}
		if err := block.Validate(); err != nil {
			return nil, fmt.Errorf("validate block %d: %w", id, err)
		}
		return block, nil
	}
	return nil, ErrBlockNotFound
}


// ReadBlocks 按区间读取连续块；任一块缺失即返回错误。
func (a *Appender) ReadBlocks(from, to uint64) ([]*model.Block, error) {
	if from > to {
		return nil, fmt.Errorf("invalid block range %d..%d", from, to)
	}
	blocks := make([]*model.Block, 0, to-from+1)
	for id := from; id <= to; id++ {
		block, err := a.ReadBlock(id)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// ReadRecord 按序号查找记录，遍历全部块定位。
func (a *Appender) ReadRecord(seq uint64) (*model.Record, error) {
	ids, err := a.ListBlockIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		block, err := a.ReadBlock(id)
		if err != nil {
			if errors.Is(err, ErrBlockNotFound) {
				continue
			}
			return nil, err
		}
		for i := range block.Records {
			rec := block.Records[i]
			if rec.Seq == seq {
				copyRecord := rec
				return &copyRecord, nil
			}
		}
	}
	return nil, fmt.Errorf("record %d not found", seq)
}

// ListBlockIDs 返回活跃区与归档区的全部块序号（升序）。
func (a *Appender) ListBlockIDs() ([]uint64, error) {
	paths := a.store.Paths()
	seen := make(map[uint64]bool)
	for _, dir := range []string{paths.BlocksDir(), paths.ArchiveDir()} {
		names, err := a.store.ListFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			id, err := parseBlockID(name)
			if err != nil {
				continue
			}
			seen[id] = true
		}
	}
	ids := make([]uint64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// BlockCount 返回活跃区块文件数量，供健康检查使用。
func (a *Appender) BlockCount() (int, error) {
	names, err := a.store.ListFiles(a.store.Paths().BlocksDir())
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

// DeleteBlock 删除块文件与对应的元数据日志；块不存在时返回 false。
func (a *Appender) DeleteBlock(id uint64) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	paths := a.store.Paths()
	removed := false
	for _, candidate := range []string{paths.BlockFile(id), paths.ArchiveBlockFile(id)} {
		if a.store.Exists(candidate) {
			if err := a.store.Remove(candidate); err != nil {
				return false, err
			}
			removed = true
		}
	}
	if err := a.store.Remove(paths.JournalFile(id)); err != nil {
		return removed, err
	}
	if removed && a.head != nil && a.head.ID == id {
		a.head = nil
		a.headFile = nil
		a.currentPath = ""
	}
	return removed, nil
}

// parseBlockID 从块文件名解析序号，非块文件返回错误。
func parseBlockID(name string) (uint64, error) {
	return store.ParseBlockFileName(name)
}
