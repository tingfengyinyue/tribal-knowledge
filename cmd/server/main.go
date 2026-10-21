package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/tingfengyinyue/tribal-knowledge/internal/config"
	"github.com/tingfengyinyue/tribal-knowledge/internal/embedding"
	"github.com/tingfengyinyue/tribal-knowledge/internal/mcpserver"
	"github.com/tingfengyinyue/tribal-knowledge/internal/retrieval"
	"github.com/tingfengyinyue/tribal-knowledge/internal/session"
	"github.com/tingfengyinyue/tribal-knowledge/internal/store"
	"github.com/tingfengyinyue/tribal-knowledge/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.Load()

	// Store
	st, err := store.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	slog.Info("database connected")

	// Embedder
	var embedder embedding.Embedder
	switch cfg.EmbeddingDriver {
	case "dashscope":
		embedder = embedding.NewDashScope(cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDim)
	case "openai":
		embedder = embedding.NewOpenAI(cfg.EmbeddingAPIKey, cfg.EmbeddingModel, "", cfg.EmbeddingDim)
	default:
		slog.Warn("no embedding driver configured, using noop embedder")
		embedder = embedding.NewNoop(cfg.EmbeddingDim)
	}

	// Services
	ret := retrieval.New(st, embedder)
	svc := session.NewService(st, ret)

	// MCP server
	mcpSrv := mcpserver.New(svc, st)

	// HTTP router
	r := gin.Default()

	// REST API
	handler := web.NewHandler(svc, st)
	handler.RegisterRoutes(r)

	// MCP endpoint
	r.Any("/mcp", web.WrapMCP(mcpSrv.Server()))

	slog.Info("starting server", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
