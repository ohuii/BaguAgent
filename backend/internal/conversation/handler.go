package conversation

import (
	"net/http"
	"strconv"

	"bagu-agent/backend/internal/message"
	"bagu-agent/backend/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 处理会话相关 HTTP 请求。
type Handler struct {
	convRepo *Repository
	msgRepo  *message.Repository
}

// NewHandler 创建会话 handler。
func NewHandler(convRepo *Repository, msgRepo *message.Repository) *Handler {
	return &Handler{convRepo: convRepo, msgRepo: msgRepo}
}

// RegisterRoutes 注册会话 API。
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/conversations", h.Create)
	r.GET("/conversations", h.List)
	r.GET("/conversations/:id/messages", h.Messages)
}

// Create 新建会话。
func (h *Handler) Create(c *gin.Context) {
	var input struct {
		UserID uint64 `json:"user_id"`
		Title  string `json:"title"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.UserID == 0 {
		input.UserID = 1
	}
	if input.Title == "" {
		input.Title = "新的面试复习会话"
	}

	conv := &Conversation{UserID: input.UserID, Title: input.Title}
	if err := h.convRepo.Create(c.Request.Context(), conv); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, conv)
}

// List 查询用户会话列表。
func (h *Handler) List(c *gin.Context) {
	userID, err := parseUintDefault(c.Query("user_id"), 1)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user_id")
		return
	}
	convs, err := h.convRepo.ListByUserID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, convs)
}

// Messages 查询某个会话的消息。
func (h *Handler) Messages(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	messages, err := h.msgRepo.ListByConversationID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, messages)
}

func parseUintDefault(raw string, defaultValue uint64) (uint64, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}
