package store

import (
	"fmt"
	"time"

	"bagu-agent/backend/internal/agent"
	"bagu-agent/backend/internal/chunk"
	"bagu-agent/backend/internal/config"
	"bagu-agent/backend/internal/conversation"
	"bagu-agent/backend/internal/document"
	"bagu-agent/backend/internal/eval"
	"bagu-agent/backend/internal/message"
	"bagu-agent/backend/internal/user"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewMySQL 创建 Gorm MySQL 连接，并配置基础连接池参数。
// 这里仅负责基础设施初始化，具体数据访问放到各业务 repository。
func NewMySQL(cfg config.MySQLConfig, log *zap.Logger) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql dsn is empty")
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	if cfg.ConnMaxLifetime != "" {
		d, err := time.ParseDuration(cfg.ConnMaxLifetime)
		if err != nil {
			return nil, fmt.Errorf("parse mysql conn max lifetime: %w", err)
		}
		sqlDB.SetConnMaxLifetime(d)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	log.Info("mysql connected")
	return db, nil
}

// AutoMigrate 在本地开发阶段自动同步表结构。
// 生产环境建议改为 migrations/ 下的 SQL 或迁移工具统一管理。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&user.User{},
		&document.Document{},
		&chunk.DocumentChunk{},
		&conversation.Conversation{},
		&message.Message{},
		&agent.AgentRun{},
		&eval.RAGEvalCase{},
		&eval.RAGEvalResult{},
	)
}
