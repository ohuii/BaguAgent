package indexer

import (
	"net/http"
	"strconv"

	"bagu-agent/backend/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 处理文档索引任务相关 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 创建索引任务 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes 注册索引任务 API。
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/documents/:id/index", h.StartDocumentIndex)
	r.GET("/index-tasks/:task_id", h.GetTask)
}

// StartDocumentIndex 创建异步索引任务。
func (h *Handler) StartDocumentIndex(c *gin.Context) {
	documentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || documentID == 0 {
		response.Error(c, http.StatusBadRequest, "invalid document id")
		return
	}

	task, err := h.service.StartIndexDocument(c.Request.Context(), documentID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, task)
}

// GetTask 查询索引任务进度。
func (h *Handler) GetTask(c *gin.Context) {
	task, err := h.service.GetTask(c.Request.Context(), c.Param("task_id"))
	if err != nil {
		response.Error(c, http.StatusNotFound, "index task not found")
		return
	}
	response.OK(c, task)
}
