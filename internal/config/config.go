package config

import "os"

type Config struct {
	Port            string
	DatabaseURL     string
	EmbeddingDriver string // openai, dashscope, or empty for no vector search
	EmbeddingAPIKey string
	EmbeddingModel  string
	EmbeddingURL    string // OpenAI-compatible base URL
	EmbeddingDim    int
	AdminToken      string // required for admin endpoints; empty disables them
}

func Load() *Config {
	c := &Config{
		Port:            envOr("TK_PORT", "8080"),
		DatabaseURL:     envOr("TK_DATABASE_URL", "postgres://tk:tk_dev@localhost:5433/tribal_knowledge?sslmode=disable"),
		EmbeddingDriver: os.Getenv("TK_EMBEDDING_DRIVER"),
		EmbeddingAPIKey: os.Getenv("TK_EMBEDDING_API_KEY"),
		EmbeddingModel:  os.Getenv("TK_EMBEDDING_MODEL"),
		EmbeddingURL:    os.Getenv("TK_EMBEDDING_URL"),
		EmbeddingDim:    1536,
		AdminToken:      os.Getenv("TK_ADMIN_TOKEN"),
	}
	switch c.EmbeddingDriver {
	case "openai":
		if c.EmbeddingModel == "" {
			c.EmbeddingModel = "text-embedding-3-small"
		}
	case "dashscope":
		if c.EmbeddingModel == "" {
			c.EmbeddingModel = "text-embedding-v3"
		}
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
