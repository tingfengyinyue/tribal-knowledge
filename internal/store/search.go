package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgvector/pgvector-go"
	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
)

type SearchParams struct {
	Tags      []string
	Embedding *pgvector.Vector
	Query     string
	ItemTypes []string
	NodeIDs   []int64
	MaxItems  int
}

type SearchResult struct {
	model.Item
	TagScore  float64 `gorm:"column:tag_score"`
	VecScore  float64 `gorm:"column:vec_score"`
	TextScore float64 `gorm:"column:text_score"`
}

func (s *Store) Search(ctx context.Context, p SearchParams) ([]SearchResult, error) {
	if p.MaxItems <= 0 {
		p.MaxItems = 30
	}

	var (
		conditions []string
		args       []any
		argIdx     int
	)
	nextArg := func(v any) string {
		argIdx++
		args = append(args, v)
		return fmt.Sprintf("$%d", argIdx)
	}

	conditions = append(conditions, "status = 'active'")

	if len(p.ItemTypes) > 0 {
		placeholders := make([]string, len(p.ItemTypes))
		for i, t := range p.ItemTypes {
			placeholders[i] = nextArg(t)
		}
		conditions = append(conditions, fmt.Sprintf("item_type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(p.NodeIDs) > 0 {
		arr := model.Int64Array(p.NodeIDs)
		v, _ := arr.Value()
		conditions = append(conditions, fmt.Sprintf("node_ids && %s::bigint[]", nextArg(v)))
	}

	// Tag score expression
	tagExpr := "0::float"
	if len(p.Tags) > 0 {
		tagLit := pgTextArrayLiteral(p.Tags)
		tagExpr = fmt.Sprintf(
			`(SELECT count(*)::float FROM unnest(tags) t WHERE lower(t) = ANY(SELECT lower(x) FROM unnest(%s::text[]) x)) / GREATEST(array_length(%s::text[], 1), 1)`,
			tagLit, tagLit)
	}

	// Vector score expression
	vecExpr := "0::float"
	if p.Embedding != nil {
		vecExpr = fmt.Sprintf("CASE WHEN embedding IS NOT NULL THEN 1 - (embedding <=> %s) ELSE 0 END", nextArg(*p.Embedding))
	}

	// Full-text score expression
	textExpr := "0::float"
	if p.Query != "" {
		tsQuery := strings.Join(strings.Fields(p.Query), " | ")
		textExpr = fmt.Sprintf("ts_rank(ts_content, to_tsquery('simple', %s))", nextArg(tsQuery))
	}

	limitArg := nextArg(p.MaxItems)

	sql := fmt.Sprintf(`
		SELECT *,
			%s AS tag_score,
			%s AS vec_score,
			%s AS text_score
		FROM tk_items
		WHERE %s
		ORDER BY (0.45 * (%s) + 0.35 * (%s) + 0.20 * (%s)) DESC
		LIMIT %s
	`, tagExpr, vecExpr, textExpr,
		strings.Join(conditions, " AND "),
		tagExpr, vecExpr, textExpr,
		limitArg)

	var results []SearchResult
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return results, nil
}

func pgTextArrayLiteral(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''"))
	}
	return "ARRAY[" + strings.Join(quoted, ",") + "]"
}
