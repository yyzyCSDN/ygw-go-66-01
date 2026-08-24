package main

import (
	"context"
	"net/http"
	"time"
)

// handleHealthz 返回健康检查结果。
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	payload := s.svc.HealthCheck(ctx)
	writeJSON(w, http.StatusOK, payload)
}

// handlePing 返回轻量探活响应。
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"pong": true})
}
