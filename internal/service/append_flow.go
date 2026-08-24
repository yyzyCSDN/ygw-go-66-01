package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"auditlog/internal/model"
)

// AppendRecord 写入一条审计记录并同步更新索引。
func (s *Service) AppendRecord(ctx context.Context, rec model.Record) (model.Record, error) {
	if err := ctx.Err(); err != nil {
		return model.Record{}, err
	}
	written, err := s.Append.Append(rec)
	if err != nil {
		return model.Record{}, err
	}
	if err := s.Index.Add(written); err != nil {
		return model.Record{}, fmt.Errorf("index record %d: %w", written.Seq, err)
	}
	if err := s.WriteJournal(written.Seq, "append"); err != nil {
		return model.Record{}, err
	}
	s.contextNote("appended record %d by %s (%s)", written.Seq, written.Actor, written.Action)
	return written, nil
}

// AppendBatch 批量写入记录，任一条失败即整体返回错误。
func (s *Service) AppendBatch(ctx context.Context, records []model.Record) ([]model.Record, error) {
	written := make([]model.Record, 0, len(records))
	for _, rec := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item, err := s.AppendRecord(ctx, rec)
		if err != nil {
			return nil, err
		}
		written = append(written, item)
	}
	return written, nil
}

// WriteJournal 把服务级操作写入块元数据日志。
func (s *Service) WriteJournal(blockID uint64, action string) error {
	entry := model.JournalEntry{
		BlockID: blockID,
		Action:  action,
		At:      time.Now().UTC(),
		Meta:    action,
	}
	path := s.Store.Paths().JournalFile(blockID)
	existing, err := s.Store.ReadAll(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read journal %d: %w", blockID, err)
		}
		existing = nil
	}
	data := append(existing, entry.Encode()...)
	if err := s.Store.WriteFile(path, data); err != nil {
		return fmt.Errorf("write journal %d: %w", blockID, err)
	}
	return nil
}

// ReadJournal 读取指定块的全部元数据日志条目。
func (s *Service) ReadJournal(blockID uint64) ([]model.JournalEntry, error) {
	path := s.Store.Paths().JournalFile(blockID)
	data, err := s.Store.ReadAll(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries := make([]model.JournalEntry, 0)
	for len(data) > 0 {
		entry, err := model.DecodeJournalEntry(data)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		encoded := entry.Encode()
		data = data[len(encoded):]
	}
	return entries, nil
}
