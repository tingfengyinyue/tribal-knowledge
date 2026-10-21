package config

import "os"

type Config struct {
	Port            string
	DatabaseURL     string
	EmbeddingDriver string // dashscope, openai
	EmbeddingAPIKey string
	EmbeddingModel  string
	EmbeddingDim    int
}

func Load() *Config {
	c := &Config{
		Port:            envOr("TK_PORT", "8080"),
		DatabaseURL:     envOr("TK_DATABASE_URL", "postgres://tk:tk_dev@localhost:5433/tribal_knowledge?sslmode=disable"),
		EmbeddingDriver: envOr("TK_EMBEDDING_DRIVER", "openai"),
		EmbeddingAPIKey: os.Getenv("TK_EMBEDDING_API_KEY"),
		EmbeddingModel:  envOr("TK_EMBEDDING_MODEL", "text-embedding-3-small"),
		EmbeddingDim:    1536,
	}
	if c.EmbeddingDriver == "dashscope" && c.EmbeddingModel == "text-embedding-3-small" {
		c.EmbeddingModel = "text-embedding-v3"
		c.EmbeddingDim = 1024
	}
	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
