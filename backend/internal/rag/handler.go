package rag

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bagu-agent/backend/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 处理 Agent/RAG 问答请求。
type Handler struct {
	service *Service
}

// NewHandler 创建 RAG handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes 注册 Agent Chat API。
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/agent/chat", h.Chat)
	r.POST("/agent/chat/stream", h.ChatStream)
	r.POST("/agent/questions", h.GenerateQuestions)
}

// Chat 执行一次基于知识库的面试问答。
func (h *Handler) Chat(c *gin.Context) {
	var input ChatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.service.Chat(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, result)
}

// ChatStream 通过 Server-Sent Events 流式返回面试问答。
func (h *Handler) ChatStream(c *gin.Context) {
	var input ChatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	emit := func(event StreamEvent) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event.Type); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	if err := h.service.ChatStream(c.Request.Context(), input, emit); err != nil {
		_ = emit(StreamEvent{Type: "error", Error: err.Error()})
	}
}

// GenerateQuestions 根据知识点生成面试题。
func (h *Handler) GenerateQuestions(c *gin.Context) {
	var input QuestionGenerateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.service.GenerateQuestions(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, result)
}
