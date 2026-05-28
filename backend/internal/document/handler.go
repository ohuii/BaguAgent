package document

import (
	"net/http"
	"strconv"

	"bagu-agent/backend/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 处理文档相关 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 创建文档 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes 注册文档 API。
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/documents/upload", h.Upload)
	r.GET("/documents", h.List)
	r.GET("/documents/:id", h.Get)
	r.DELETE("/documents/:id", h.Delete)
	r.GET("/documents/:id/chunks", h.ListChunks)
	r.POST("/documents/:id/index", h.Index)
}

// Upload 上传并解析 Markdown 文档。
func (h *Handler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "file is required")
		return
	}

	userID, err := parseUintDefault(c.PostForm("user_id"), 1)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user_id")
		return
	}

	result, err := h.service.UploadMarkdown(c.Request.Context(), UploadMarkdownInput{
		UserID:   userID,
		Category: c.PostForm("category"),
		File:     file,
	})
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	response.OK(c, result)
}

// List 查询文档列表。
func (h *Handler) List(c *gin.Context) {
	userID, err := parseUintDefault(c.Query("user_id"), 1)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user_id")
		return
	}

	docs, err := h.service.List(c.Request.Context(), userID, c.Query("category"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, docs)
}

// Get 查询文档详情。
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	doc, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusNotFound, "document not found")
		return
	}
	response.OK(c, doc)
}

// Delete 删除文档和 MySQL chunk。
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// ListChunks 查询某个文档的 chunk。
func (h *Handler) ListChunks(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	chunks, err := h.service.ListChunks(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, chunks)
}

// Index 是第三阶段 Milvus 索引接口的占位实现。
func (h *Handler) Index(c *gin.Context) {
	response.Error(c, http.StatusNotImplemented, "document indexing will be implemented in phase 3")
}

func parseIDParam(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func parseUintDefault(raw string, defaultValue uint64) (uint64, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}
