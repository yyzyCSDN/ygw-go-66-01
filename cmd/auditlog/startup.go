package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"auditlog/internal/reten"
	"auditlog/internal/service"
)

// runServer 启动 HTTP 服务并在上下文取消时优雅退出。
func runServer(ctx context.Context, cfg *Config, svc *service.Service) error {
	policy, err := reten.ParsePolicy(cfg.Retention)
	if err != nil {
		return err
	}
	server := &Server{
		cfg: cfg,
		svc: svc,
		http: &http.Server{
			Addr:              cfg.Addr,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       90 * time.Second,
		},
	}
	mux := http.NewServeMux()
	server.registerRoutes(mux)
	server.http.Handler = mux

	maintenanceCtx, cancelMaintenance := context.WithCancel(ctx)
	defer cancelMaintenance()
	go func() {
		_ = svc.RunMaintenanceLoop(maintenanceCtx, cfg.RetentionInterval, policy)
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.http.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.http.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
