package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bagu-agent/backend/internal/config"
	"bagu-agent/backend/internal/pkg/logger"
	"bagu-agent/backend/internal/router"
	"bagu-agent/backend/internal/store"

	"go.uber.org/zap"
)

// main 负责组装应用依赖并启动 HTTP 服务。
// 业务模块不要直接写在这里，后续只在这里增加初始化和依赖注入。
func main() {
	cfg, err := config.Load("")
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = log.Sync()
	}()

	db, err := store.NewMySQL(cfg.MySQL, log)
	if err != nil {
		log.Fatal("connect mysql failed", zap.Error(err))
	}

	if cfg.App.AutoMigrate {
		if err := store.AutoMigrate(db); err != nil {
			log.Fatal("auto migrate failed", zap.Error(err))
		}
	}

	engine := router.New(router.Dependencies{
		Config: cfg,
		Logger: log,
		DB:     db,
	})

	srv := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("server started", zap.String("addr", cfg.Server.Addr()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server stopped unexpectedly", zap.Error(err))
		}
	}()

	// 监听退出信号，给正在处理的请求留出优雅关闭时间。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server shutdown failed", zap.Error(err))
		return
	}
	log.Info("server stopped")
}
