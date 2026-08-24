package export

import (
	"fmt"
	"os"
	"strings"

	"auditlog/internal/model"
)

// saveCursor 持久化导出游标。
func (e *Exporter) saveCursor(cursor *model.ExportCursor) error {
	if cursor == nil {
		return fmt.Errorf("cannot save a nil export cursor")
	}
	path := e.store.Paths().CursorFile(cursor.ExporterID)
	if err := e.store.WriteFile(path, cursor.Encode()); err != nil {
		return fmt.Errorf("save export cursor: %w", err)
	}
	return nil
}

// loadCursor 读取导出游标。
func (e *Exporter) loadCursor(id string) (*model.ExportCursor, error) {
	path := e.store.Paths().CursorFile(id)
	data, err := e.store.ReadAll(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("export %s not found", id)
		}
		return nil, err
	}
	cursor, err := model.DecodeExportCursor(data)
	if err != nil {
		return nil, fmt.Errorf("decode export cursor %s: %w", id, err)
	}
	return &cursor, nil
}

// ListExports 返回全部导出任务的游标文件（按任务 ID 排序）。
func (e *Exporter) ListExports() ([]string, error) {
	names, err := e.store.ListFiles(e.store.Paths().CursorDir())
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasSuffix(name, ".cur") {
			ids = append(ids, strings.TrimSuffix(name, ".cur"))
		}
	}
	return ids, nil
}
