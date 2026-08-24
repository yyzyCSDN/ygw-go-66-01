package reten

import (
	"fmt"

	"auditlog/internal/model"
)

// listBlocks 返回活跃区与归档区的全部块序号。
func (r *Retention) listBlocks() ([]uint64, error) {
	paths := r.store.Paths()
	seen := make(map[uint64]bool)
	for _, dir := range []string{paths.BlocksDir(), paths.ArchiveDir()} {
		names, err := r.store.ListFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			id, err := parseBlockFileName(name)
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
	sortIDs(ids)
	return ids, nil
}

// loadBlock 从磁盘读取块内容。
func (r *Retention) loadBlock(id uint64) (*model.Block, error) {
	paths := r.store.Paths()
	for _, candidate := range []string{paths.BlockFile(id), paths.ArchiveBlockFile(id)} {
		if !r.store.Exists(candidate) {
			continue
		}
		data, err := r.store.ReadAll(candidate)
		if err != nil {
			return nil, err
		}
		return model.DecodeBlock(data)
	}
	return nil, nil
}

// deleteBlock 删除块文件与对应的元数据日志。
func (r *Retention) deleteBlock(id uint64) error {
	paths := r.store.Paths()
	removed := false
	for _, candidate := range []string{paths.BlockFile(id), paths.ArchiveBlockFile(id)} {
		if r.store.Exists(candidate) {
			if err := r.store.Remove(candidate); err != nil {
				return err
			}
			removed = true
		}
	}
	if err := r.store.Remove(paths.JournalFile(id)); err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("block %d has no files to delete", id)
	}
	return nil
}

// parseBlockFileName 从文件名解析块序号。
func parseBlockFileName(name string) (uint64, error) {
	var id uint64
	count := 0
	for _, ch := range name {
		if ch < '0' || ch > '9' {
			break
		}
		id = id*10 + uint64(ch-'0')
		count++
	}
	if count != 20 || name[count:] != ".blk" {
		return 0, fmt.Errorf("not a block file: %s", name)
	}
	return id, nil
}

func sortIDs(ids []uint64) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
