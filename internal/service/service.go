package service

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"auditlog/internal/append"
	"auditlog/internal/archive"
	"auditlog/internal/chain"
	"auditlog/internal/export"
	"auditlog/internal/index"
	"auditlog/internal/reten"
	"auditlog/internal/search"
	"auditlog/internal/store"
	"auditlog/internal/verify"
)

// Config 描述服务的运行配置。
type Config struct {
	DataDir           string
	BlockSize         int
	RetentionPolicy   string
	RetentionInterval time.Duration
	ExportLimit       int
	Logger            *log.Logger
}

// DefaultConfig 返回带默认值的配置。
func DefaultConfig() Config {
	return Config{
		DataDir:           "data",
		BlockSize:         append.DefaultBlockSize,
		RetentionPolicy:   "30d",
		RetentionInterval: time.Hour,
		ExportLimit:       100,
		Logger:            log.New(os.Stderr, "[auditlog] ", log.LstdFlags),
	}
}

// Stats 是健康检查与状态页面使用的运行指标快照。
type Stats struct {
	UptimeSeconds   int64
	BlockCount      int
	HeadBlockID     uint64
	NextSeq         uint64
	RecordTotal     int
	OpenFiles       int
	OpenFilePaths   []string
	ArchivedBlocks  int
	ChainGeneration uint64
	AppendTotal     uint64
	AppendErrors    uint64
}

// Service 聚合各组件，向 HTTP 层提供统一入口。
type Service struct {
	cfg     Config
	logger  *log.Logger
	lock    *store.DirLock
	Store   *store.FileStore
	Chain   *chain.Chain
	Append  *append.Appender
	Index   *index.Index
	Search  *search.Searcher
	Archive *archive.Archiver
	Reten   *reten.Retention
	Export  *export.Exporter
	Verify  *verify.Verifier

	startedAt  time.Time
	mu         sync.Mutex
	lastVerify *verify.Report
}

// NewService 装配全部组件并从数据目录恢复状态。
func NewService(cfg Config) (*Service, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = DefaultConfig().Logger
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory %s: %w", cfg.DataDir, err)
	}
	lock, err := store.AcquireDirLock(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	cleanup := func(service *Service) {
		_ = lock.Release()
		if service != nil && service.Store != nil {
			_ = service.Store.Close()
		}
	}
	fileStore, err := store.NewFileStore(cfg.DataDir)
	if err != nil {
		cleanup(nil)
		return nil, err
	}
	chainCore := chain.NewChain()
	appender, err := append.NewAppender(fileStore, chainCore, cfg.BlockSize, logger)
	if err != nil {
		cleanup(nil)
		return nil, err
	}
	idx := index.NewIndex()
	searcher, err := search.NewSearcher(idx, appender)
	if err != nil {
		cleanup(nil)
		return nil, err
	}
	archiver, err := archive.NewArchiver(fileStore, logger)
	if err != nil {
		cleanup(nil)
		return nil, err
	}
	retention := reten.NewRetention(fileStore, archiver, appender, logger)
	exporter := export.NewExporter(fileStore, idx, appender, logger)
	verifier, err := verify.NewVerifier(chainCore, appender, logger)
	if err != nil {
		cleanup(nil)
		return nil, err
	}
	service := &Service{
		cfg:       cfg,
		logger:    logger,
		lock:      lock,
		Store:     fileStore,
		Chain:     chainCore,
		Append:    appender,
		Index:     idx,
		Search:    searcher,
		Archive:   archiver,
		Reten:     retention,
		Export:    exporter,
		Verify:    verifier,
		startedAt: time.Now().UTC(),
	}
	if err := service.restoreIndexSnapshot(); err != nil {
		cleanup(service)
		return nil, err
	}
	return service, nil
}

// Close 释放目录锁并关闭全部句柄。
func (s *Service) Close() error {
	var first error
	if err := s.Store.Close(); err != nil && first == nil {
		first = err
	}
	if err := s.lock.Release(); err != nil && first == nil {
		first = err
	}
	return first
}

// Stats 返回当前运行指标。
func (s *Service) Stats() Stats {
	blockCount, _ := s.Append.BlockCount()
	archived := s.Archive.ManifestSnapshot().Count()
	return Stats{
		UptimeSeconds:   int64(time.Since(s.startedAt).Seconds()),
		BlockCount:      blockCount,
		HeadBlockID:     s.Append.CurrentBlockID(),
		NextSeq:         s.Append.NextSeq(),
		RecordTotal:     s.Index.Total(),
		OpenFiles:       s.Store.OpenFileCount(),
		OpenFilePaths:   s.Store.TrackedPaths(),
		ArchivedBlocks:  archived,
		ChainGeneration: s.Index.Generation(),
		AppendTotal:     s.Append.SnapshotMetrics().AppendTotal,
		AppendErrors:    s.Append.SnapshotMetrics().AppendErrors,
	}
}

// Config 返回服务配置。
func (s *Service) Config() Config {
	return s.cfg
}

// LastVerify 返回最近一次完整性校验报告。
func (s *Service) LastVerify() *verify.Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastVerify
}

// setLastVerify 保存最近一次校验报告。
func (s *Service) setLastVerify(report *verify.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastVerify = report
}

// restoreIndexSnapshot 从磁盘恢复索引快照。
func (s *Service) restoreIndexSnapshot() error {
	path := s.Store.Paths().IndexFile()
	if !s.Store.Exists(path) {
		return nil
	}
	data, err := s.Store.ReadAll(path)
	if err != nil {
		return fmt.Errorf("read index snapshot: %w", err)
	}
	snapshot, err := index.DecodeSnapshot(data)
	if err != nil {
		return fmt.Errorf("decode index snapshot: %w", err)
	}
	if err := s.Index.Restore(snapshot); err != nil {
		return err
	}
	s.logger.Printf("restored index snapshot generation %d with %d records", snapshot.Generation, snapshot.Total)
	return nil
}

// snapshotIndex 把当前索引写入磁盘。
func (s *Service) snapshotIndex() error {
	snapshot := s.Index.Snapshot()
	data := index.EncodeSnapshot(snapshot)
	if err := s.Store.WriteFile(s.Store.Paths().IndexFile(), data); err != nil {
		return fmt.Errorf("save index snapshot: %w", err)
	}
	return nil
}

// ParseRetentionPolicy 解析配置中的保留策略。
func (s *Service) ParseRetentionPolicy() (reten.Policy, error) {
	return reten.ParsePolicy(s.cfg.RetentionPolicy)
}

// contextNote 记录一条服务级操作日志。
func (s *Service) contextNote(format string, args ...any) {
	s.logger.Printf(format, args...)
}
