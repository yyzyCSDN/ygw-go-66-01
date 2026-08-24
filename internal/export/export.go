package export

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"auditlog/internal/index"
	"auditlog/internal/model"
	"auditlog/internal/search"
	"auditlog/internal/store"
)

// ErrExportComplete 表示导出任务已全部完成。
var ErrExportComplete = errors.New("export already complete")

// Exporter 以游标方式顺序导出审计记录，支持中断后续传。
type Exporter struct {
	store  *store.FileStore
	index  *index.Index
	reader search.RecordReader
	logger *log.Logger
}

// NewExporter 创建导出器。
func NewExporter(st *store.FileStore, idx *index.Index, reader search.RecordReader, logger *log.Logger) *Exporter {
	if logger == nil {
		logger = log.New(os.Stderr, "[export] ", log.LstdFlags)
	}
	return &Exporter{store: st, index: idx, reader: reader, logger: logger}
}

// Start 启动一次新导出：记录当前索引代次并持久化游标。
func (e *Exporter) Start() (*model.ExportCursor, error) {
	id := fmt.Sprintf("exp-%d", time.Now().UnixNano())
	snapshot := e.index.Snapshot()
	cursor := &model.ExportCursor{
		ExporterID: id,
		LastSeq:    0,
		BlockID:    0,
		IndexSeq:   snapshot.Generation,
		UpdatedAt:  time.Now().UTC(),
		Complete:   false,
	}
	if err := e.saveCursor(cursor); err != nil {
		return nil, err
	}
	return cursor, nil
}

// Next 推进游标并返回下一批记录。记录序号在服务内连续，按序号顺序读取。
func (e *Exporter) Next(cursor *model.ExportCursor, limit int) ([]model.Record, error) {
	if cursor == nil {
		return nil, errors.New("export cursor is nil")
	}
	if limit <= 0 {
		limit = 100
	}
	if cursor.Complete {
		return nil, ErrExportComplete
	}
	bound := e.exportBound(cursor)
	records := make([]model.Record, 0, limit)
	last := cursor.LastSeq
	for seq := cursor.LastSeq + 1; seq <= bound && len(records) < limit; seq++ {
		rec, err := e.reader.ReadRecord(seq)
		if err != nil {
			continue
		}
		records = append(records, *rec)
		last = seq
	}
	cursor.LastSeq = last
	cursor.UpdatedAt = time.Now().UTC()
	if cursor.LastSeq >= bound {
		cursor.Complete = true
	}
	if err := e.saveCursor(cursor); err != nil {
		return nil, err
	}
	return records, nil
}

// Resume 按任务 ID 恢复导出游标，并从最新索引代次继续。
func (e *Exporter) Resume(id string) (*model.ExportCursor, error) {
	cursor, err := e.loadCursor(id)
	if err != nil {
		return nil, err
	}
	if cursor.Complete {
		return cursor, nil
	}
	previous := cursor.IndexSeq
	snapshot := e.index.Snapshot()
	cursor.IndexSeq = snapshot.Generation
	if err := e.saveCursor(cursor); err != nil {
		return nil, err
	}
	pending := e.index.RecordsAfter(previous)
	e.logger.Printf("resumed export %s at seq %d, %d new records pending",
		id, cursor.LastSeq, len(pending))
	return cursor, nil
}

// Finish 将导出任务标记为完成。
func (e *Exporter) Finish(id string) error {
	cursor, err := e.loadCursor(id)
	if err != nil {
		return err
	}
	cursor.Complete = true
	cursor.UpdatedAt = time.Now().UTC()
	return e.saveCursor(cursor)
}

// exportBound 计算本次导出应推进到的记录序号上界。
func (e *Exporter) exportBound(cursor *model.ExportCursor) uint64 {
	return uint64(e.index.Total())
}
