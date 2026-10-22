package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tingfengyinyue/tribal-knowledge/internal/bootstrap"
	"github.com/tingfengyinyue/tribal-knowledge/internal/embedding"
	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
	"github.com/tingfengyinyue/tribal-knowledge/internal/retrieval"
	"github.com/tingfengyinyue/tribal-knowledge/internal/store"
)

type Service struct {
	store     *store.Store
	retriever *retrieval.Retriever
	embedder  embedding.Embedder // nil: no vector search
	now       func() time.Time
}

func NewService(s *store.Store, r *retrieval.Retriever, e embedding.Embedder) *Service {
	return &Service{store: s, retriever: r, embedder: e, now: time.Now}
}

// Open creates a new session, retrieves relevant knowledge, and returns the initial context.
func (svc *Service) Open(ctx context.Context, in OpenInput) (*OpenOutput, error) {
	gitRemote := normalizeRepo(in.Client.GitRemote)

	sess := &model.Session{
		Question:         in.Question,
		ClientName:       in.Client.Name,
		BootstrapVersion: in.Client.BootstrapVersion,
		GitRemote:        gitRemote,
		Status:           "open",
	}
	if err := svc.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	maxItems := in.MaxItems
	if maxItems <= 0 {
		maxItems = 30
	}

	// Load always-on instructions
	instructions, err := svc.store.ListItemsByType(ctx, "instruction")
	if err != nil {
		slog.WarnContext(ctx, "load instructions failed", "error", err)
	}
	var instrData []InstructionItem
	for _, it := range instructions {
		if it.Status == "active" && it.LoadWhen == "always" {
			instrData = append(instrData, InstructionItem{Title: it.Title, Content: it.Content})
		}
	}

	// Parse question (simplified: extract tags from question)
	tags := extractTags(in.Question)

	// Search relevant items
	ranked, err := svc.retriever.Search(ctx, in.Question, tags, nil, nil, maxItems)
	if err != nil {
		slog.WarnContext(ctx, "retrieval failed", "error", err)
	}

	items := make([]ItemView, len(ranked))
	for i, r := range ranked {
		items[i] = ItemView{
			ID:           r.ID,
			ItemType:     r.ItemType,
			Title:        r.Title,
			Summary:      r.Summary,
			Brief:        r.Brief,
			Score:        r.Score,
			Trust:        r.Trust,
			LastVerified: r.LastVerified,
			Stale:        r.Stale,
		}
	}

	// Repo context
	var repoCtx *RepoContext
	if gitRemote != "" {
		repoCtx = svc.repoContext(ctx, gitRemote)
	}

	return &OpenOutput{
		SessionID:        sess.ID,
		BootstrapVersion: bootstrap.Version,
		Instruction:      InstructionSet{Data: instrData},
		Parsed:           ParsedQuestion{Services: nil, Identifiers: map[string][]string{}},
		Graph:            GraphView{},
		Items:            items,
		Notices:          nil,
		RepoContext:      repoCtx,
	}, nil
}

// Close ends a session, applying conclusion downgrade if no evidence.
func (svc *Service) Close(ctx context.Context, in CloseInput) (*CloseOutput, error) {
	sess, err := svc.store.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if sess.Status == "closed" {
		return nil, fmt.Errorf("session already closed")
	}

	steps, err := svc.store.ListSteps(ctx, sess.ID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}

	status := in.Conclusion.Status
	var warnings []CloseWarning

	// Evidence-based downgrade
	if !hasQueryEvidence(steps) {
		downgraded := map[string]string{
			"confirmed": "likely",
			"answered":  "blocked",
		}
		if newStatus, ok := downgraded[status]; ok {
			warnings = append(warnings, CloseWarning{
				Field:   "conclusion.status",
				Message: fmt.Sprintf("downgraded from %q to %q: no evidence steps in this session", status, newStatus),
			})
			status = newStatus
		}
	}

	sess.Status = "closed"
	sess.QuestionAbstract = in.QuestionAbstract
	sess.Conclusion = model.JSONB{
		"status":      status,
		"summary":     in.Conclusion.Summary,
		"root_causes": in.Conclusion.RootCauses,
		"identifiers": in.Conclusion.Identifiers,
	}
	if err := svc.store.UpdateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("close session: %w", err)
	}

	return &CloseOutput{Status: status, Warnings: warnings}, nil
}

// LogStep records a step in the session.
func (svc *Service) LogStep(ctx context.Context, in LogStepInput) (*LogStepOutput, error) {
	resourceKey := ""
	if in.Resource != nil {
		resourceKey = in.Resource.NodeKey
	}

	idsFound := model.JSONB{}
	for k, v := range in.IDsFound {
		idsFound[k] = v
	}

	step := &model.Step{
		SessionID:   in.SessionID,
		Seq:         in.Seq,
		StepType:    in.StepType,
		Tool:        in.Tool,
		Command:     in.Command,
		ResourceKey: resourceKey,
		ResultClass: in.ResultClass,
		RowsCount:   in.Rows,
		Summary:     in.Summary,
		IDsFound:    idsFound,
		UsedItemIDs: model.Int64Array(in.UsedItemIDs),
	}
	if err := svc.store.UpsertStep(ctx, step); err != nil {
		return nil, fmt.Errorf("upsert step: %w", err)
	}

	// Generate hints
	var hints []string
	if in.StepType == "query" && in.ResultClass == "no_data" {
		hints = append(hints, "consider checking if the resource key or time range is correct")
	}

	return &LogStepOutput{Hints: hints}, nil
}

// GetSession returns a compressed view of the session for reflection.
func (svc *Service) GetSession(ctx context.Context, sessionID int64) (*GetSessionOutput, error) {
	sess, err := svc.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	steps, err := svc.store.ListSteps(ctx, sess.ID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}

	views := make([]StepView, len(steps))
	for i, s := range steps {
		views[i] = StepView{
			Seq:         s.Seq,
			StepType:    s.StepType,
			Tool:        s.Tool,
			ResourceKey: s.ResourceKey,
			ResultClass: s.ResultClass,
			Summary:     s.Summary,
		}
	}

	return &GetSessionOutput{
		SessionID: sess.ID,
		Question:  sess.Question,
		Steps:     views,
	}, nil
}

// SearchItems searches the knowledge base directly (used by skill_search).
func (svc *Service) SearchItems(ctx context.Context, query string, maxItems int) ([]ItemView, error) {
	if maxItems <= 0 {
		maxItems = 30
	}
	tags := extractTags(query)
	ranked, err := svc.retriever.Search(ctx, query, tags, nil, nil, maxItems)
	if err != nil {
		return nil, err
	}
	items := make([]ItemView, len(ranked))
	for i, r := range ranked {
		items[i] = ItemView{
			ID:           r.ID,
			ItemType:     r.ItemType,
			Title:        r.Title,
			Summary:      r.Summary,
			Brief:        r.Brief,
			Score:        r.Score,
			Trust:        r.Trust,
			LastVerified: r.LastVerified,
			Stale:        r.Stale,
		}
	}
	return items, nil
}

func extractTags(question string) []string {
	// Simplified tag extraction — in production, use NLP or LLM
	return nil
}
