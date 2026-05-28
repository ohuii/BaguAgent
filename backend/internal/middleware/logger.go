package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ZapLogger 记录每次 HTTP 请求的核心访问信息。
func ZapLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		log.Info("http request",
			zap.String("request_id", requestID(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Int("size", c.Writer.Size()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// ZapRecovery 捕获 panic 并写入结构化日志，避免服务进程直接退出。
func ZapRecovery(log *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic recovered",
			zap.String("request_id", requestID(c)),
			zap.Any("error", recovered),
		)
		c.AbortWithStatus(500)
	})
}

// requestID 从 Gin Context 中读取请求 ID，供日志中间件复用。
func requestID(c *gin.Context) string {
	v, ok := c.Get(RequestIDKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
