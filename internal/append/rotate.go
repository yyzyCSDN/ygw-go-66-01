package append

import (
	"fmt"
	"time"

	"auditlog/internal/model"
)

// Rotate 按时间执行日志轮转：为下一周期创建新块并切换写句柄。
// 轮转由后台任务在每日边界触发，保证新周期日志写入新块文件。
func (a *Appender) Rotate(now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.head == nil {
		return nil
	}
	next := a.head.ID + 1
	block := &model.Block{
		ID:        next,
		CreatedAt: now,
		Records:   nil,
		State:     model.StateActive,
	}
	if err := a.chain.Link(a.head, block); err != nil {
		return err
	}
	path := a.store.Paths().BlockFile(block.ID)
	file, err := a.store.OpenAppend(path)
	if err != nil {
		return fmt.Errorf("open rotation block file: %w", err)
	}
	block.File = path
	a.head = block
	a.headFile = file
	a.currentPath = path
	if err := a.store.CloseAllExcept(path); err != nil {
		return fmt.Errorf("close stale block files: %w", err)
	}
	a.metrics.BlocksSealed++
	a.metrics.RotationsDone++
	a.logger.Printf("rotated to block %d, open files: %v", block.ID, a.store.TrackedPaths())
	return nil
}
