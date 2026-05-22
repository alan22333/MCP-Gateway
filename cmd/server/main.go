// MCP Nexus — AI 模型上下文协议网关
//
// 启动流程：
//   bootstrap() → 装配所有依赖（配置、数据库、缓存、服务、handler）
//   registerRoutes() → 注册 4 个路由分组（观测、MCP 协议、管理 API、前端）
//   ListenAndServe → 优雅关闭
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func main() {
	deps, err := bootstrap()
	if err != nil {
		panic("bootstrap failed: " + err.Error())
	}
	defer close(deps.done)
	defer deps.logger.Sync()

	// Session expiration cleanup
	go deps.sessionMgr.CleanupExpired(30*time.Minute, deps.done)
	deps.logger.Info("session cleanup started", zap.Duration("ttl", 30*time.Minute))

	// Routes
	r := registerRoutes(deps)
	addr := ":" + strconv.Itoa(deps.cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	// Start
	go func() {
		deps.logger.Info("MCP Nexus starting",
			zap.String("address", addr),
			zap.String("dashboard", "http://localhost"+addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			deps.logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	deps.logger.Info("shutting down...", zap.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		deps.logger.Error("shutdown timeout", zap.Error(err))
	}
	if sqlDB, err := deps.db.DB(); err == nil {
		sqlDB.Close()
	}
	deps.grpcProxy.Close()
	deps.logger.Info("server stopped")
}
