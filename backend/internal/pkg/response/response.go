package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 是 API 的统一响应结构，方便前端稳定解析。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Error 返回错误响应。
func Error(c *gin.Context, status int, message string) {
	c.JSON(status, Body{
		Code:    status,
		Message: message,
	})
}
