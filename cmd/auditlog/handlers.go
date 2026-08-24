package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"auditlog/internal/append"
	"auditlog/internal/chain"
	"auditlog/internal/export"
	"auditlog/internal/index"
	"auditlog/internal/model"
	"auditlog/internal/reten"
	"auditlog/internal/search"
	"auditlog/internal/service"
	"auditlog/internal/verify"
)

// Server 提供 HTTP 处理器。
type Server struct {
	cfg  *Config
	svc  *service.Service
	http *http.Server
}

// registerRoutes 注册全部路由。
func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleConsole)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/v1/ping", s.handlePing)
	mux.HandleFunc("/api/v1/records/batch", s.handleAppendBatch)
	mux.HandleFunc("/api/v1/records/filter", s.handleFilter)
	mux.HandleFunc("/api/v1/records/next-seq", s.handleNextSeq)
	mux.HandleFunc("/api/v1/records", s.handleRecords)
	mux.HandleFunc("/api/v1/records/", s.handleRecordBySeq)
	mux.HandleFunc("/api/v1/verify", s.handleVerify)
	mux.HandleFunc("/api/v1/verify/range", s.handleVerifyRange)
	mux.HandleFunc("/api/v1/archive", s.handleArchive)
	mux.HandleFunc("/api/v1/retention", s.handleRetention)
	mux.HandleFunc("/api/v1/export", s.handleExport)
	mux.HandleFunc("/api/v1/export/verify", s.handleExportVerify)
	mux.HandleFunc("/api/v1/export/", s.handleExportByID)
	mux.HandleFunc("/api/v1/journal/", s.handleJournal)
	mux.HandleFunc("/api/v1/blocks", s.handleBlocks)
	mux.HandleFunc("/api/v1/blocks/", s.handleBlockByID)
	mux.HandleFunc("/api/v1/rotate", s.handleRotate)
	mux.HandleFunc("/api/v1/index/snapshot", s.handleIndexSnapshot)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
}

// handleConsole 返回控制台页面。
func (s *Server) handleConsole(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/console") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	path := filepath.Join(s.cfg.WebRoot, "console.html")
	data, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "console page unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// handleRecords 追加或检索记录。
func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleAppend(w, r)
	case http.MethodGet:
		s.handleSearch(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAppend 追加一条审计记录。
func (s *Server) handleAppend(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	var request struct {
		Actor     string    `json:"actor"`
		Action    string    `json:"action"`
		Detail    string    `json:"detail"`
		WrittenAt time.Time `json:"written_at"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	rec := model.Record{
		Actor:     request.Actor,
		Action:    request.Action,
		Detail:    request.Detail,
		WrittenAt: request.WrittenAt,
	}
	written, err := s.svc.AppendRecord(r.Context(), rec)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, written)
}

// handleSearch 按条件检索并分页。
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := index.Query{
		Actor:   r.URL.Query().Get("actor"),
		Action:  r.URL.Query().Get("action"),
		Keyword: r.URL.Query().Get("keyword"),
	}
	if from := r.URL.Query().Get("from"); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from time")
			return
		}
		query.From = parsed
	}
	if to := r.URL.Query().Get("to"); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to time")
			return
		}
		query.To = parsed
	}
	page, err := intQuery(r, "page", 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page")
		return
	}
	size, err := intQuery(r, "size", 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid size")
		return
	}
	result, err := s.svc.SearchRecords(r.Context(), query, page, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRecordBySeq 读取单条记录。
func (s *Server) handleRecordBySeq(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/v1/records/")
	seq, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid record seq")
		return
	}
	rec, err := s.svc.GetRecord(r.Context(), seq)
	if err != nil {
		if errors.Is(err, search.ErrRecordNotFound) {
			writeError(w, http.StatusNotFound, "record not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleNextSeq 返回下一条记录的预分配序号。
func (s *Server) handleNextSeq(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	next, err := s.svc.NextSequence(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next_seq": next})
}

// handleAppendBatch 批量追加审计记录。
func (s *Server) handleAppendBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	var request struct {
		Records []model.Record `json:"records"`
	}
	if err := json.Unmarshal(body, &request); err != nil || len(request.Records) == 0 {
		writeError(w, http.StatusBadRequest, "records array is required")
		return
	}
	written, err := s.svc.AppendBatch(r.Context(), request.Records)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"count": len(written), "first_seq": written[0].Seq, "last_seq": written[len(written)-1].Seq})
}

// handleFilter 对指定记录做二次过滤。
func (s *Server) handleFilter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	var request struct {
		Seqs    []uint64 `json:"seqs"`
		Actor   string   `json:"actor"`
		Action  string   `json:"action"`
		Keyword string   `json:"keyword"`
	}
	if err := json.Unmarshal(body, &request); err != nil || len(request.Seqs) == 0 {
		writeError(w, http.StatusBadRequest, "seqs array is required")
		return
	}
	records, err := s.svc.FilterRecordsBySeq(r.Context(), request.Seqs, request.Actor, request.Action, request.Keyword, time.Time{}, time.Time{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(records), "records": records})
}

// handleVerify 执行完整性校验。
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var report *verify.Report
	switch r.Method {
	case http.MethodGet:
		report = s.svc.LastVerify()
		if report == nil {
			writeError(w, http.StatusNotFound, "no verification has run")
			return
		}
	case http.MethodPost:
		var err error
		report, err = s.svc.VerifyAll(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload := map[string]any{
		"checked": report.Checked,
		"broken":  report.Broken,
		"passed":  report.Passed,
		"ran_at":  report.RanAt,
		"summary": report.Summary(),
		"digest":  chain.DigestHex(report.RecordsDigest),
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleVerifyRange 校验指定区间的块。
func (s *Server) handleVerifyRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	var request struct {
		From uint64 `json:"from"`
		To   uint64 `json:"to"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.From == 0 || request.To < request.From {
		writeError(w, http.StatusBadRequest, "from/to block range is required")
		return
	}
	report, err := s.svc.VerifyRange(r.Context(), request.From, request.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"checked": report.Checked,
		"broken":  report.Broken,
		"passed":  report.Passed,
		"summary": report.Summary(),
	})
}

// handleArchive 按窗口归档。
func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	var request struct {
		Size int `json:"size"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.Size <= 0 {
		writeError(w, http.StatusBadRequest, "archive size must be positive")
		return
	}
	archived, err := s.svc.ArchiveWindow(r.Context(), request.Size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"archived": archived, "count": len(archived)})
}

// handleRetention 应用保留策略。
func (s *Server) handleRetention(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	var request struct {
		Policy string `json:"policy"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	var policy reten.Policy
	if request.Policy == "" {
		policy, err = s.svc.ParseRetentionPolicy()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		policy, err = reten.ParsePolicy(request.Policy)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	report, err := s.svc.ApplyRetention(r.Context(), policy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// handleExport 启动或推进导出。
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		cursor, err := s.svc.ExportStart(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, cursor)
	case http.MethodGet:
		ids, err := s.svc.Export.ListExports()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"exports": ids, "count": len(ids)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleExportByID 按任务 ID 恢复并推进导出。
func (s *Server) handleExportByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/export/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "export id is required")
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(id, "/finish") {
		id = strings.TrimSuffix(id, "/finish")
		if err := s.svc.Export.Finish(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"export_id": id, "complete": true})
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, err := intQuery(r, "limit", s.cfg.ExportLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	cursor, err := s.svc.ExportResume(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	records, err := s.svc.ExportNext(r.Context(), cursor, limit)
	if err != nil {
		if errors.Is(err, append.ErrBlockNotFound) {
			writeError(w, http.StatusConflict, "export source block missing")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cursor":  cursor,
		"records": records,
		"format":  "line",
		"lines":   string(export.FormatExport(records)),
	})
}

// handleExportVerify 校验导出行内容。
func (s *Server) handleExportVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	valid := 0
	var firstError string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if _, err := export.ParseExportLine(line); err != nil {
			if firstError == "" {
				firstError = err.Error()
			}
			continue
		}
		valid++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lines":   len(lines),
		"valid":   valid,
		"invalid": len(lines) - valid,
		"error":   firstError,
	})
}

// handleJournal 读取指定块的元数据日志。
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/v1/journal/")
	blockID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid block id")
		return
	}
	entries, err := s.svc.Journal(r.Context(), blockID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"block_id": blockID, "entries": entries})
}

// handleBlocks 列出或按区间读取块。
func (s *Server) handleBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	fromRaw := r.URL.Query().Get("from")
	toRaw := r.URL.Query().Get("to")
	if fromRaw != "" && toRaw != "" {
		from, err := strconv.ParseUint(fromRaw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from")
			return
		}
		to, err := strconv.ParseUint(toRaw, 10, 64)
		if err != nil || to < from {
			writeError(w, http.StatusBadRequest, "invalid to")
			return
		}
		blocks, err := s.svc.ReadBlockRange(r.Context(), from, to)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks, "count": len(blocks)})
		return
	}
	ids, err := s.svc.ListBlocks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"block_ids": ids, "count": len(ids)})
}

// handleBlockByID 读取或删除单个块。
func (s *Server) handleBlockByID(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/api/v1/blocks/")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid block id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		block, err := s.svc.ReadBlock(r.Context(), id)
		if err != nil {
			if errors.Is(err, append.ErrBlockNotFound) {
				writeError(w, http.StatusNotFound, "block not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, block)
	case http.MethodDelete:
		removed, err := s.svc.DeleteBlock(r.Context(), id)
		if err != nil {
			if errors.Is(err, append.ErrBlockNotFound) {
				writeError(w, http.StatusNotFound, "block not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"removed": removed, "block_id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRotate 立即执行日志轮转。
func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.svc.RotateLogs(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rotated": true})
}

// handleIndexSnapshot 立即持久化索引快照。
func (s *Server) handleIndexSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.svc.SnapshotIndexNow(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshotted": true})
}

// handleStats 返回运行指标。
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.svc.Stats())
}

func intQuery(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
