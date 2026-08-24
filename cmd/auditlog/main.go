package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"auditlog/internal/service"
)

func main() {
	logger := log.New(os.Stderr, "[auditlog] ", log.LstdFlags)
	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}
	serviceConfig := service.Config{
		DataDir:           cfg.DataDir,
		BlockSize:         cfg.BlockSize,
		RetentionPolicy:   cfg.Retention,
		RetentionInterval: cfg.RetentionInterval,
		ExportLimit:       cfg.ExportLimit,
		Logger:            logger,
	}
	svc, err := service.NewService(serviceConfig)
	if err != nil {
		logger.Fatalf("initialize service: %v", err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			logger.Printf("close service: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runServer(ctx, cfg, svc); err != nil {
		logger.Printf("server stopped: %v", err)
	}
	logger.Printf("auditlog stopped")
}
