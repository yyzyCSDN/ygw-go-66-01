package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths 定义数据目录下的全部子目录与固定文件位置。
type Paths struct {
	Root string
}

// BlocksDir 返回活跃块文件目录。
func (p Paths) BlocksDir() string {
	return filepath.Join(p.Root, "blocks")
}

// ArchiveDir 返回归档块文件目录。
func (p Paths) ArchiveDir() string {
	return filepath.Join(p.Root, "archive")
}

// JournalDir 返回块元数据日志目录。
func (p Paths) JournalDir() string {
	return filepath.Join(p.Root, "journal")
}

// MetaDir 返回服务元数据目录。
func (p Paths) MetaDir() string {
	return filepath.Join(p.Root, "meta")
}

// CursorDir 返回导出游标目录。
func (p Paths) CursorDir() string {
	return filepath.Join(p.Root, "cursor")
}

// IndexFile 返回索引快照文件路径。
func (p Paths) IndexFile() string {
	return filepath.Join(p.Root, "index.snap")
}

// HeadFile 返回当前头块记录文件路径。
func (p Paths) HeadFile() string {
	return filepath.Join(p.MetaDir(), "head.bin")
}

// BlockFile 返回指定序号块的文件路径。
func (p Paths) BlockFile(id uint64) string {
	return filepath.Join(p.BlocksDir(), fmt.Sprintf("%020d.blk", id))
}

// ArchiveBlockFile 返回指定序号块在归档区的文件路径。
func (p Paths) ArchiveBlockFile(id uint64) string {
	return filepath.Join(p.ArchiveDir(), fmt.Sprintf("%020d.blk", id))
}

// JournalFile 返回指定块对应的元数据日志文件路径。
func (p Paths) JournalFile(id uint64) string {
	return filepath.Join(p.JournalDir(), fmt.Sprintf("%020d.jrn", id))
}

// CursorFile 返回指定导出任务的游标文件路径。
func (p Paths) CursorFile(id string) string {
	return filepath.Join(p.CursorDir(), id+".cur")
}

// EnsureDirs 创建数据目录的全部子目录。
func EnsureDirs(root string) error {
	paths := Paths{Root: root}
	for _, dir := range []string{
		paths.BlocksDir(),
		paths.ArchiveDir(),
		paths.JournalDir(),
		paths.MetaDir(),
		paths.CursorDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}
