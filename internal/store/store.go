package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileStore 负责块文件与元数据文件的读写。追加打开的写句柄会被登记，
// 供健康检查统计打开文件数；读取始终使用独立句柄。
type FileStore struct {
	paths Paths
	mu    sync.Mutex
	open  map[string]*os.File
}

// NewFileStore 打开数据目录并登记路径。
func NewFileStore(root string) (*FileStore, error) {
	if err := EnsureDirs(root); err != nil {
		return nil, err
	}
	return &FileStore{
		paths: Paths{Root: root},
		open:  make(map[string]*os.File),
	}, nil
}

// Paths 返回目录布局。
func (s *FileStore) Paths() Paths {
	return s.paths
}

// Exists 判断文件是否存在。
func (s *FileStore) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadAll 读取整个文件内容。
func (s *FileStore) ReadAll(path string) ([]byte, error) {
	file, err := s.OpenRead(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// WriteFile 以临时文件加重命名的方式原子写入文件内容。
func (s *FileStore) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	temporary, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	temporaryName := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}
	if _, err := temporary.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary file %s: %w", temporaryName, err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary file %s: %w", temporaryName, err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("close temporary file %s: %w", temporaryName, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("rename temporary file to %s: %w", path, err)
	}
	return nil
}

// OpenRead 打开一个只读句柄，不登记到打开句柄表。
func (s *FileStore) OpenRead(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open read %s: %w", path, err)
	}
	return file, nil
}

// OpenAppend 打开一个读写句柄并登记到句柄表；同一路径重复打开会先关闭旧句柄。
func (s *FileStore) OpenAppend(path string) (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous := s.open[path]; previous != nil {
		_ = previous.Close()
		delete(s.open, path)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open append %s: %w", path, err)
	}
	s.open[path] = file
	return file, nil
}

// SyncPath 对指定路径执行落盘同步；未打开的路径直接跳过。
func (s *FileStore) SyncPath(path string) error {
	s.mu.Lock()
	file := s.open[path]
	s.mu.Unlock()
	if file == nil {
		return nil
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	return nil
}

// CloseFile 关闭指定路径的登记句柄。
func (s *FileStore) CloseFile(path string) error {
	s.mu.Lock()
	file := s.open[path]
	delete(s.open, path)
	s.mu.Unlock()
	if file == nil {
		return nil
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

// Remove 删除文件；同时清理该路径的登记句柄。
func (s *FileStore) Remove(path string) error {
	s.mu.Lock()
	if file := s.open[path]; file != nil {
		_ = file.Close()
		delete(s.open, path)
	}
	s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// OpenFileCount 返回当前登记的打开写句柄数量。
func (s *FileStore) OpenFileCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.open)
}

// TrackedPaths 返回当前登记的全部打开写句柄路径。
func (s *FileStore) TrackedPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	paths := make([]string, 0, len(s.open))
	for path := range s.open {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// CloseAllExcept 关闭全部登记句柄，仅保留 keep 中列出的路径。
// 轮转切换写句柄时用于清理旧文件句柄，避免句柄泄漏。
func (s *FileStore) CloseAllExcept(keep ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keepSet := make(map[string]bool, len(keep))
	for _, path := range keep {
		keepSet[path] = true
	}
	var first error
	for path, file := range s.open {
		if keepSet[path] {
			continue
		}
		if err := file.Close(); err != nil && first == nil {
			first = fmt.Errorf("close %s: %w", path, err)
		}
		delete(s.open, path)
	}
	return first
}

// ListFiles 返回目录下按字典序排列的文件名。
func (s *FileStore) ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list directory %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Close 关闭全部登记句柄。
func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var first error
	for path, file := range s.open {
		if err := file.Close(); err != nil && first == nil {
			first = fmt.Errorf("close %s: %w", path, err)
		}
	}
	s.open = make(map[string]*os.File)
	return first
}

// ParseBlockFileName 从块文件名解析块序号；非法名称返回错误。
func ParseBlockFileName(name string) (uint64, error) {
	base := strings.TrimSuffix(name, ".blk")
	if base == name || len(base) != 20 {
		return 0, fmt.Errorf("invalid block file name %q", name)
	}
	var id uint64
	for _, ch := range base {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid block file name %q", name)
		}
		id = id*10 + uint64(ch-'0')
	}
	return id, nil
}
