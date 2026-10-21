package retrieval

import (
	"context"
	"log/slog"
	"time"

	"github.com/pgvector/pgvector-go"
	"github.com/tingfengyinyue/tribal-knowledge/internal/embedding"
	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
	"github.com/tingfengyinyue/tribal-knowledge/internal/store"
)

const (
	weightTag  = 0.45
	weightVec  = 0.35
	weightText = 0.20

	staleAfter   = 90 * 24 * time.Hour
	penaltyStale = 0.8
)

type RankedItem struct {
	ID           int64   `json:"id"`
	ItemType     string  `json:"item_type"`
	Title        string  `json:"title"`
	Summary      string  `json:"summary"`
	Brief        string  `json:"brief,omitempty"`
	Score        float64 `json:"score"`
	Trust        float32 `json:"trust"`
	LastVerified string  `json:"last_verified_at,omitempty"`
	Stale        bool    `json:"stale,omitempty"`
}

type Retriever struct {
	store    *store.Store
	embedder embedding.Embedder
	now      func() time.Time
}

func New(s *store.Store, e embedding.Embedder) *Retriever {
	return &Retriever{store: s, embedder: e, now: time.Now}
}

func (r *Retriever) Search(ctx context.Context, query string, tags []string, itemTypes []string, nodeIDs []int64, maxItems int) ([]RankedItem, error) {
	start := time.Now()

	var vec *pgvector.Vector
	if r.embedder != nil {
		vecs, err := r.embedder.Embed(ctx, []string{query})
		if err != nil {
			slog.WarnContext(ctx, "embedding failed, falling back to text+tag search", "error", err)
		} else if len(vecs) > 0 {
			v := pgvector.NewVector(vecs[0])
			vec = &v
		}
	}

	results, err := r.store.Search(ctx, store.SearchParams{
		Tags:      tags,
		Embedding: vec,
		Query:     query,
		ItemTypes: itemTypes,
		NodeIDs:   nodeIDs,
		MaxItems:  maxItems * 2, // fetch more, then re-rank
	})
	if err != nil {
		return nil, err
	}

	now := r.now()
	ranked := make([]RankedItem, 0, len(results))
	for _, sr := range results {
		score := weightTag*sr.TagScore + weightVec*sr.VecScore + weightText*sr.TextScore
		score *= float64(sr.Item.Trust)

		var lastVerified string
		var stale bool
		if t, ok := model.VerifiedAt(&sr.Item); ok {
			lastVerified = t.Format(time.RFC3339)
			if now.Sub(t) > staleAfter {
				stale = true
				score *= penaltyStale
			}
		} else {
			if now.Sub(sr.Item.UpdatedAt) > staleAfter {
				stale = true
				score *= penaltyStale
			}
		}

		brief := sr.Item.Summary
		if len(brief) > 200 {
			brief = brief[:200] + "…"
		}

		ranked = append(ranked, RankedItem{
			ID:           sr.Item.ID,
			ItemType:     sr.Item.ItemType,
			Title:        sr.Item.Title,
			Summary:      sr.Item.Summary,
			Brief:        brief,
			Score:        score,
			Trust:        sr.Item.Trust,
			LastVerified: lastVerified,
			Stale:        stale,
		})
	}

	// Already sorted by DB, but trim to maxItems
	if len(ranked) > maxItems {
		ranked = ranked[:maxItems]
	}

	slog.InfoContext(ctx, "skill search",
		"query_len", len(query),
		"tags", len(tags),
		"results", len(ranked),
		"duration", time.Since(start).String())

	return ranked, nil
}
