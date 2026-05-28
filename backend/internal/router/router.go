package router

import (
	"net/http"

	chunkrepo "bagu-agent/backend/internal/chunk"
	"bagu-agent/backend/internal/config"
	"bagu-agent/backend/internal/document"
	"bagu-agent/backend/internal/embedder"
	"bagu-agent/backend/internal/indexer"
	"bagu-agent/backend/internal/middleware"
	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Dependencies 是路由层需要的外部依赖。
// 后续 handler 会从这里拿 service，而不是直接操作数据库。
type Dependencies struct {
	Config *config.Config
	Logger *zap.Logger
	DB     *gorm.DB
}

// New 创建 Gin Engine 并注册全局中间件和 API 路由。
func New(deps Dependencies) *gin.Engine {
	gin.SetMode(deps.Config.Server.Mode)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.ZapRecovery(deps.Logger))
	r.Use(middleware.ZapLogger(deps.Logger))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"app":    deps.Config.App.Name,
		})
	})

	api := r.Group("/api")
	{
		api.GET("/healthz", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		docRepo := document.NewRepository(deps.DB)
		chunkRepo := chunkrepo.NewRepository(deps.DB)
		embeddingClient, err := embedder.New(deps.Config.AI, deps.Config.Milvus.EmbeddingDim)
		if err != nil {
			panic(err)
		}
		milvusStore := vectorstore.NewLazyMilvusStore(deps.Config.Milvus)
		indexerService := indexer.NewService(deps.Config.Milvus.CollectionName, docRepo, chunkRepo, embeddingClient, milvusStore)
		docService := document.NewService(deps.Config.Storage, docRepo, chunkRepo, indexerService)
		document.NewHandler(docService).RegisterRoutes(api)
		retriever.NewHandler(retriever.NewService(embeddingClient, milvusStore)).RegisterRoutes(api)
	}

	return r
}
