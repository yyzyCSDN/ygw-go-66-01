package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DirLock 是基于独占创建语义的轻量目录锁，用于防止同一数据目录被多个进程打开。
type DirLock struct {
	path string
	held bool
}

// AcquireDirLock 尝试在目标目录创建锁文件；锁已存在且超过陈旧阈值时接管。
func AcquireDirLock(root string) (*DirLock, error) {
	lockPath := filepath.Join(root, ".auditlog.lock")
	deadline := time.Now().Add(-2 * time.Minute)
	for attempt := 0; attempt < 3; attempt++ {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = file.WriteString(fmt.Sprintf("%d\n", time.Now().Unix()))
			_ = file.Close()
			return &DirLock{path: lockPath, held: true}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}
		info, statErr := os.Stat(lockPath)
		if statErr == nil && info.ModTime().Before(deadline) {
			_ = os.Remove(lockPath)
			continue
		}
		return nil, fmt.Errorf("data directory %s is locked by another process", root)
	}
	return nil, fmt.Errorf("data directory %s is locked and stale lock could not be reclaimed", root)
}

// Release 释放目录锁。
func (l *DirLock) Release() error {
	if l == nil || !l.held {
		return nil
	}
	l.held = false
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("release lock %s: %w", l.path, err)
	}
	return nil
}
