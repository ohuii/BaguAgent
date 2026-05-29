package eval

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bagu-agent/backend/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

// NewHandler 创建 RAG 评测 HTTP handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes 注册 RAG 评测相关 API。
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/eval/cases", h.CreateCase)
	r.GET("/eval/cases", h.ListCases)
	r.POST("/eval/run", h.Run)
	r.GET("/eval/results", h.ListResults)
}

// CreateCase 新增一条人工标注评测用例。
func (h *Handler) CreateCase(c *gin.Context) {
	var input CreateCaseInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.service.CreateCase(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, result)
}

// ListCases 查询评测用例，支持 category 和 ids 查询参数。
func (h *Handler) ListCases(c *gin.Context) {
	ids, err := parseIDList(c.Query("ids"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid ids")
		return
	}
	result, err := h.service.ListCases(c.Request.Context(), CaseFilter{
		IDs:      ids,
		Category: c.Query("category"),
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, result)
}

// Run 执行一轮检索评测并返回汇总指标。
func (h *Handler) Run(c *gin.Context) {
	var input RunInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.service.Run(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, result)
}

// ListResults 查询历史评测结果。
func (h *Handler) ListResults(c *gin.Context) {
	evalCaseID, err := parseUintDefault(c.Query("case_id"), 0)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid case_id")
		return
	}
	limit, err := parseIntDefault(c.Query("limit"), 50)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid limit")
		return
	}
	result, err := h.service.ListResults(c.Request.Context(), ResultFilter{
		EvalCaseID: evalCaseID,
		Limit:      limit,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, result)
}

// parseIDList 解析形如 "1,2,3" 的 id 查询参数。
func parseIDList(raw string) ([]uint64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]uint64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseUintDefault 解析 uint64 参数，空字符串时返回默认值。
func parseUintDefault(raw string, defaultValue uint64) (uint64, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

// parseIntDefault 解析 int 参数，空字符串时返回默认值。
func parseIntDefault(raw string, defaultValue int) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(raw)
}
