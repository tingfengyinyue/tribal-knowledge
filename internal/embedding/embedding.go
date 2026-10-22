package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
}

// OpenAI-compatible embedder (works with OpenAI, Azure, local models, etc.)
type openaiEmbedder struct {
	apiKey  string
	model   string
	baseURL string
	dim     int
	client  *http.Client
}

func NewOpenAI(apiKey, model, baseURL string, dim int) Embedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &openaiEmbedder{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		dim:     dim,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *openaiEmbedder) Dim() int { return e.dim }

func (e *openaiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]any{
		"model": e.model,
		"input": texts,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embedding API %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse embedding response: %w", err)
	}

	vecs := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// DashScope embedder (Alibaba Cloud)
type dashscopeEmbedder struct {
	apiKey string
	model  string
	dim    int
	client *http.Client
}

func NewDashScope(apiKey, model string, dim int) Embedder {
	if model == "" {
		model = "text-embedding-v3"
	}
	if dim == 0 {
		dim = 1024
	}
	return &dashscopeEmbedder{
		apiKey: apiKey,
		model:  model,
		dim:    dim,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *dashscopeEmbedder) Dim() int { return e.dim }

func (e *dashscopeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]any{
		"model": e.model,
		"input": map[string]any{
			"texts": texts,
		},
		"parameters": map[string]any{
			"dimension": e.dim,
		},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding",
		bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dashscope request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("dashscope %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Output struct {
			Embeddings []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"embeddings"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse dashscope response: %w", err)
	}

	vecs := make([][]float32, len(result.Output.Embeddings))
	for i, d := range result.Output.Embeddings {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}
