package service

import (
	"context"
	"errors"
	"time"

	appender "auditlog/internal/append"
	"auditlog/internal/index"
	"auditlog/internal/model"
	"auditlog/internal/search"
)

// SearchRecords 执行检索并分页。
func (s *Service) SearchRecords(ctx context.Context, q index.Query, page, size int) (search.PageResult, error) {
	if err := ctx.Err(); err != nil {
		return search.PageResult{}, err
	}
	return s.Search.Page(q, page, size)
}

// GetRecord 按序号读取单条记录。
func (s *Service) GetRecord(ctx context.Context, seq uint64) (*model.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Search.BySeq(seq)
}

// ListBlocks 返回全部块序号。
func (s *Service) ListBlocks(ctx context.Context) ([]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Append.ListBlockIDs()
}

// ReadBlock 按序号读取块。
func (s *Service) ReadBlock(ctx context.Context, id uint64) (*model.Block, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Append.ReadBlock(id)
}

// ReadBlockRange 按区间读取连续块。
func (s *Service) ReadBlockRange(ctx context.Context, from, to uint64) ([]*model.Block, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Append.ReadBlocks(from, to)
}

// DeleteBlock 手动删除指定块并清理头块状态。
func (s *Service) DeleteBlock(ctx context.Context, id uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	removed, err := s.Append.DeleteBlock(id)
	if err != nil {
		return false, err
	}
	if !removed {
		return false, appender.ErrBlockNotFound
	}
	return true, nil
}

// NextSequence 返回下一条记录的预分配序号。
func (s *Service) NextSequence(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.Append.NextSeq(), nil
}

// Journal 返回指定块的服务级操作日志。
func (s *Service) Journal(ctx context.Context, blockID uint64) ([]model.JournalEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := s.ReadJournal(blockID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("journal not found")
	}
	return entries, nil
}

// FilterRecordsBySeq 按序号加载记录并在内存中应用二次过滤。
func (s *Service) FilterRecordsBySeq(ctx context.Context, seqs []uint64, actor, action, keyword string, from, to time.Time) ([]model.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records := make([]model.Record, 0, len(seqs))
	for _, seq := range seqs {
		rec, err := s.Search.BySeq(seq)
		if err != nil {
			continue
		}
		records = append(records, *rec)
	}
	return search.ApplyFilters(records, actor, action, keyword, from, to), nil
}
