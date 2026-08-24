package service

import (
	"context"
	"time"

	"auditlog/internal/chain"
	"auditlog/internal/model"
	"auditlog/internal/reten"
	"auditlog/internal/verify"
)

// VerifyAll 执行全链完整性校验并保存最近报告。
func (s *Service) VerifyAll(ctx context.Context) (*verify.Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report, err := s.Verify.VerifyAll()
	if err != nil {
		return nil, err
	}
	s.setLastVerify(report)
	s.contextNote("verification finished: %s (digest %s)", report.Summary(), chain.DigestHex(report.RecordsDigest))
	return report, nil
}

// VerifyRange 校验指定区间的块。
func (s *Service) VerifyRange(ctx context.Context, from, to uint64) (*verify.Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report, err := s.Verify.VerifyRange(from, to)
	if err != nil {
		return nil, err
	}
	s.setLastVerify(report)
	return report, nil
}

// ApplyRetention 应用保留策略并返回清理报告。
func (s *Service) ApplyRetention(ctx context.Context, policy reten.Policy) (reten.Report, error) {
	if err := ctx.Err(); err != nil {
		return reten.Report{}, err
	}
	report, err := s.Reten.Apply(policy)
	if err != nil {
		return reten.Report{}, err
	}
	s.contextNote("retention applied: scanned %d, deleted %d, skipped archived %d",
		report.Scanned, report.Deleted, report.SkippedArchived)
	return report, nil
}

// ArchiveWindow 归档从最旧块开始的 size 个块，返回归档块序号。
func (s *Service) ArchiveWindow(ctx context.Context, size int) ([]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids, err := s.Append.ListBlockIDs()
	if err != nil {
		return nil, err
	}
	blocks := make([]*model.Block, 0, len(ids))
	for _, id := range ids {
		block, err := s.Append.ReadBlock(id)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	selected, err := s.Archive.Window(blocks, size)
	if err != nil {
		return nil, err
	}
	if err := s.Archive.Archive(selected); err != nil {
		return nil, err
	}
	archived := make([]uint64, 0, len(selected))
	for _, block := range selected {
		archived = append(archived, block.ID)
	}
	return archived, nil
}

// ExportStart 启动一次导出。
func (s *Service) ExportStart(ctx context.Context) (*model.ExportCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Export.Start()
}

// ExportNext 推进导出游标。
func (s *Service) ExportNext(ctx context.Context, cursor *model.ExportCursor, limit int) ([]model.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Export.Next(cursor, limit)
}

// ExportResume 恢复导出游标。
func (s *Service) ExportResume(ctx context.Context, id string) (*model.ExportCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Export.Resume(id)
}

// RunMaintenanceLoop 周期性执行保留清理与索引快照，直到上下文取消。
func (s *Service) RunMaintenanceLoop(ctx context.Context, interval time.Duration, policy reten.Policy) error {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			report, err := s.ApplyRetention(ctx, policy)
			if err != nil {
				s.contextNote("maintenance retention failed: %v", err)
				continue
			}
			if report.Deleted > 0 {
				s.contextNote("maintenance expired %d blocks", report.Deleted)
			}
			if err := s.snapshotIndex(); err != nil {
				s.contextNote("maintenance index snapshot failed: %v", err)
				continue
			}
		}
	}
}

// SnapshotIndexNow 立即持久化索引快照。
func (s *Service) SnapshotIndexNow(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.snapshotIndex()
}

// RotateLogs 立即执行日志轮转。
func (s *Service) RotateLogs(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Append.Rotate(time.Now().UTC())
}

// HealthCheck 返回服务健康描述。
func (s *Service) HealthCheck(ctx context.Context) map[string]any {
	stats := s.Stats()
	return map[string]any{
		"status":     "ok",
		"uptime_sec": stats.UptimeSeconds,
		"blocks":     stats.BlockCount,
		"records":    stats.RecordTotal,
		"open_files": stats.OpenFiles,
		"archived":   stats.ArchivedBlocks,
		"data_dir":   s.cfg.DataDir,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	}
}
