package search

import (
	"errors"
	"fmt"

	"auditlog/internal/index"
	"auditlog/internal/model"
)

// ErrRecordNotFound 表示记录不存在。
var ErrRecordNotFound = errors.New("record not found")

// RecordReader 抽象记录与块读取能力，由追加器实现。
type RecordReader interface {
	ReadRecord(seq uint64) (*model.Record, error)
	ReadBlock(id uint64) (*model.Block, error)
}

// Searcher 组合索引与读取器提供检索能力。
type Searcher struct {
	index  *index.Index
	reader RecordReader
}

// NewSearcher 创建检索器。
func NewSearcher(idx *index.Index, reader RecordReader) (*Searcher, error) {
	if idx == nil {
		return nil, errors.New("search index is required")
	}
	if reader == nil {
		return nil, errors.New("search reader is required")
	}
	return &Searcher{index: idx, reader: reader}, nil
}

// Page 按条件检索并分页。page 从 1 开始，size 为每页条数。
func (s *Searcher) Page(q index.Query, page, size int) (PageResult, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	hits, _, err := s.index.Query(q)
	if err != nil {
		return PageResult{}, err
	}
	offset := (page - 1) * size
	if offset >= len(hits) {
		return PageResult{Records: []model.Record{}, Total: len(hits), Page: page, Size: size, HasMore: false}, nil
	}
	end := offset + size
	if end > len(hits) {
		end = len(hits) - 1
	}
	selected := hits[offset:end]
	records := make([]model.Record, 0, len(selected))
	for _, seq := range selected {
		rec, err := s.reader.ReadRecord(seq)
		if err != nil {
			if errors.Is(err, ErrRecordNotFound) {
				continue
			}
			return PageResult{}, fmt.Errorf("load record %d: %w", seq, err)
		}
		if rec == nil {
			continue
		}
		records = append(records, *rec)
	}
	return PageResult{
		Records: records,
		Total:   len(hits),
		Page:    page,
		Size:    size,
		HasMore: end < len(hits),
	}, nil
}

// BySeq 按序号读取单条记录。
func (s *Searcher) BySeq(seq uint64) (*model.Record, error) {
	rec, err := s.reader.ReadRecord(seq)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, ErrRecordNotFound
	}
	return rec, nil
}

