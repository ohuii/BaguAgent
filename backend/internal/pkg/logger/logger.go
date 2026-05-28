package logger

import (
	"fmt"
	"strings"

	"bagu-agent/backend/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 根据配置创建 zap logger。
// console 适合本地开发，json/production 适合容器日志采集。
func New(cfg config.LogConfig) (*zap.Logger, error) {
	zapCfg := zap.NewProductionConfig()
	if strings.EqualFold(cfg.Encoding, "console") {
		zapCfg = zap.NewDevelopmentConfig()
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.DisableStacktrace = !cfg.Stacktrace
	zapCfg.EncoderConfig.TimeKey = "ts"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapCfg.Build()
}
