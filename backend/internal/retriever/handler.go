package retriever

import (
	"net/http"

	"bagu-agent/backend/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 处理检索调试接口。
type Handler struct {
	service *Service
}

// NewHandler 创建检索 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes 注册检索 API。
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/retrieval/search", h.Search)
}

// Search 提供 Milvus TopK 检索调试接口。
func (h *Handler) Search(c *gin.Context) {
	var input SearchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}
	results, err := h.service.Search(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, results)
}
