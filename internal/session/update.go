package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/pgvector/pgvector-go"
	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
)

// evidenceClasses are the result classes that count as a real observation.
var evidenceClasses = map[string]bool{"ok": true, "no_data": true, "wrong_mapping": true}

// Update processes feedback, reflection and operations for a session.
func (svc *Service) Update(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
	sess, err := svc.store.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if in.UpdateID != "" {
		if existing, err := svc.store.GetUpdateByKey(ctx, sess.ID, in.UpdateID); err == nil {
			var results []OpResult
			if err := json.Unmarshal(existing.Results, &results); err != nil {
				return nil, fmt.Errorf("replay update %q: %w", in.UpdateID, err)
			}
			return &UpdateOutput{Results: results}, nil
		}
	}

	steps, err := svc.store.ListSteps(ctx, sess.ID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	hasEvidence := hasQueryEvidence(steps)

	// Feedback only refreshes freshness when this session observed something real.
	if hasEvidence {
		var helpfulIDs []int64
		for _, f := range in.Feedback {
			if f.Verdict == "helpful" {
				helpfulIDs = append(helpfulIDs, f.ItemID)
			}
		}
		if err := svc.store.TouchVerified(ctx, helpfulIDs, svc.now()); err != nil {
			slog.WarnContext(ctx, "refresh last_verified_at failed", "error", err)
		}
	}

	results := make([]OpResult, len(in.Operations))
	for i, op := range in.Operations {
		results[i] = svc.processOp(ctx, sess, steps, hasEvidence, op)
	}

	reflection := model.JSONB{}
	if in.Reflection != nil {
		reflection = model.JSONB{
			"redundant_steps": in.Reflection.RedundantSteps,
			"better_order":    in.Reflection.BetterOrder,
			"pitfalls":        in.Reflection.Pitfalls,
			"next_time":       in.Reflection.NextTime,
		}
	}
	update := &model.Update{
		SessionID:  sess.ID,
		UpdateID:   in.UpdateID,
		Feedback:   model.ToJSONArray(in.Feedback),
		Reflection: reflection,
		Operations: model.ToJSONArray(in.Operations),
		Results:    model.ToJSONArray(results),
	}
	if err := svc.store.CreateUpdate(ctx, update); err != nil {
		return nil, fmt.Errorf("record update: %w", err)
	}

	return &UpdateOutput{Results: results}, nil
}

func (svc *Service) processOp(ctx context.Context, sess *model.Session, steps []model.Step, hasEvidence bool, op Operation) OpResult {
	switch op.Op {
	case "add_item", "update_item", "invalidate_item", "save_case",
		"upsert_node", "update_node", "invalidate_node", "add_edge", "invalidate_edge", "register_repo":
	default:
		return OpResult{Op: op.Op, Status: "invalid", Message: "unknown operation"}
	}

	// A session that observed nothing can only submit feedback and reflection.
	if !hasEvidence {
		return needsEvidenceResult(op.Op)
	}

	// update_item without cited evidence becomes a conflicting note instead of an overwrite.
	if op.Op == "update_item" {
		return svc.updateItem(ctx, sess, validEvidenceSteps(op.EvidenceSteps, steps), op)
	}
	if !validEvidenceSteps(op.EvidenceSteps, steps) {
		return needsEvidenceResult(op.Op)
	}

	switch op.Op {
	case "add_item":
		return svc.addItem(ctx, sess, op)
	case "invalidate_item":
		return svc.invalidateItem(ctx, sess, op)
	case "save_case":
		return svc.saveCase(ctx, sess, steps, op)
	case "upsert_node":
		return svc.upsertNode(ctx, op)
	case "update_node":
		return svc.updateNode(ctx, op)
	case "invalidate_node":
		return svc.invalidateNode(ctx, op)
	case "add_edge":
		return svc.addEdge(ctx, op)
	case "invalidate_edge":
		return svc.invalidateEdge(ctx, op)
	default: // register_repo
		return svc.registerRepo(ctx, op)
	}
}

func (svc *Service) addItem(ctx context.Context, sess *model.Session, op Operation) OpResult {
	if op.ItemType == "" || op.ItemType == "instruction" {
		return OpResult{Op: op.Op, Status: "forbidden", Message: "item_type is required; instructions are admin-only"}
	}
	item := &model.Item{
		ItemType:   op.ItemType,
		Title:      op.Title,
		Summary:    op.Content,
		Content:    op.Content,
		Tags:       model.StringArray(op.Tags),
		Attributes: model.JSONB{"recorded_at": svc.now().Format(time.RFC3339), model.AttrLastVerifiedAt: svc.now().Format(time.RFC3339)},
		Trust:      1.0,
		Status:     "active",
		LoadWhen:   "relevant",
	}
	svc.embed(ctx, item)
	if err := svc.store.CreateItem(ctx, item); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	svc.recordHistory(ctx, sess.ID, item.ID, "created", nil, item)
	return OpResult{Op: op.Op, Status: "created", ItemID: item.ID}
}

func (svc *Service) updateItem(ctx context.Context, sess *model.Session, cited bool, op Operation) OpResult {
	existing, err := svc.store.GetItem(ctx, op.ItemID)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
	}
	if existing.ItemType == "instruction" {
		return OpResult{Op: op.Op, Status: "forbidden", Message: "instructions are admin-only"}
	}

	if !cited {
		conflict := &model.Item{
			ItemType:   existing.ItemType,
			Title:      op.Title,
			Summary:    op.Content,
			Content:    op.Content,
			Tags:       model.StringArray(op.Tags),
			Attributes: model.JSONB{"conflict_with": existing.ID, "recorded_at": svc.now().Format(time.RFC3339)},
			Trust:      0.5,
			Status:     "active",
			LoadWhen:   "relevant",
		}
		if conflict.Title == "" {
			conflict.Title = existing.Title
		}
		svc.embed(ctx, conflict)
		if err := svc.store.CreateItem(ctx, conflict); err != nil {
			return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
		}
		svc.recordHistory(ctx, sess.ID, conflict.ID, "created", nil, conflict)
		return OpResult{Op: op.Op, Status: "created_as_conflict", ItemID: conflict.ID}
	}

	before := itemToJSON(existing)
	if op.Title != "" {
		existing.Title = op.Title
	}
	if op.Content != "" {
		existing.Content = op.Content
		existing.Summary = op.Content
	}
	if op.Tags != nil {
		existing.Tags = model.StringArray(op.Tags)
	}
	if existing.Attributes == nil {
		existing.Attributes = model.JSONB{}
	}
	existing.Attributes[model.AttrLastVerifiedAt] = svc.now().Format(time.RFC3339)
	svc.embed(ctx, existing)
	if err := svc.store.UpdateItem(ctx, existing); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	svc.recordHistory(ctx, sess.ID, existing.ID, "updated", before, existing)
	return OpResult{Op: op.Op, Status: "updated", ItemID: existing.ID}
}

func (svc *Service) invalidateItem(ctx context.Context, sess *model.Session, op Operation) OpResult {
	existing, err := svc.store.GetItem(ctx, op.ItemID)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
	}
	if existing.ItemType == "instruction" {
		return OpResult{Op: op.Op, Status: "forbidden", Message: "instructions are admin-only"}
	}
	before := itemToJSON(existing)
	existing.Status = "invalid"
	if err := svc.store.UpdateItem(ctx, existing); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	svc.recordHistory(ctx, sess.ID, existing.ID, "invalidated", before, existing)
	return OpResult{Op: op.Op, Status: "invalidated", ItemID: existing.ID}
}

func (svc *Service) upsertNode(ctx context.Context, op Operation) OpResult {
	if op.NodeKey == "" || op.NodeType == "" {
		return OpResult{Op: op.Op, Status: "invalid", Message: "node_key and node_type are required"}
	}
	node := &model.Node{
		NodeKey:    op.NodeKey,
		NodeType:   op.NodeType,
		Name:       op.Name,
		Attributes: model.JSONB(op.Attributes),
	}
	if err := svc.store.UpsertNode(ctx, node); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	return OpResult{Op: op.Op, Status: "created", NodeKey: node.NodeKey}
}

func (svc *Service) updateNode(ctx context.Context, op Operation) OpResult {
	node, err := svc.store.GetNode(ctx, op.NodeKey)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
	}
	if op.Name != "" {
		node.Name = op.Name
	}
	if node.Attributes == nil {
		node.Attributes = model.JSONB{}
	}
	for k, v := range op.Attributes {
		if v != nil {
			node.Attributes[k] = v
		}
	}
	if err := svc.store.UpdateNode(ctx, node); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	return OpResult{Op: op.Op, Status: "updated", NodeKey: node.NodeKey}
}

func (svc *Service) invalidateNode(ctx context.Context, op Operation) OpResult {
	node, err := svc.store.GetNode(ctx, op.NodeKey)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
	}
	node.Status = "invalid"
	if err := svc.store.UpdateNode(ctx, node); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	return OpResult{Op: op.Op, Status: "invalidated", NodeKey: node.NodeKey}
}

func (svc *Service) edgeEnds(ctx context.Context, op Operation) (src, dst *model.Node, res *OpResult) {
	src, err := svc.store.GetNode(ctx, op.SourceNodeKey)
	if err != nil {
		return nil, nil, &OpResult{Op: op.Op, Status: "not_found", Message: "source node not found: " + op.SourceNodeKey}
	}
	dst, err = svc.store.GetNode(ctx, op.TargetNodeKey)
	if err != nil {
		return nil, nil, &OpResult{Op: op.Op, Status: "not_found", Message: "target node not found: " + op.TargetNodeKey}
	}
	return src, dst, nil
}

func (svc *Service) addEdge(ctx context.Context, op Operation) OpResult {
	src, dst, res := svc.edgeEnds(ctx, op)
	if res != nil {
		return *res
	}
	edge := &model.Edge{SourceNodeID: src.ID, TargetNodeID: dst.ID, EdgeType: op.EdgeType}
	if err := svc.store.UpsertEdge(ctx, edge); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	return OpResult{Op: op.Op, Status: "created"}
}

func (svc *Service) invalidateEdge(ctx context.Context, op Operation) OpResult {
	src, dst, res := svc.edgeEnds(ctx, op)
	if res != nil {
		return *res
	}
	n, err := svc.store.InvalidateEdge(ctx, src.ID, dst.ID, op.EdgeType)
	if err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	if n == 0 {
		return OpResult{Op: op.Op, Status: "not_found", Message: "no active edge matches"}
	}
	return OpResult{Op: op.Op, Status: "invalidated"}
}

func (svc *Service) saveCase(ctx context.Context, sess *model.Session, steps []model.Step, op Operation) OpResult {
	snapshot := make([]map[string]any, len(steps))
	for i, s := range steps {
		snapshot[i] = map[string]any{
			"seq": s.Seq, "step_type": s.StepType,
			"tool": s.Tool, "result_class": s.ResultClass,
			"summary": s.Summary,
		}
	}
	conclusion := model.JSONB{}
	if sess.Conclusion != nil {
		conclusion = sess.Conclusion
	}
	c := &model.Case{
		SessionID:     sess.ID,
		Title:         sess.QuestionAbstract,
		Summary:       fmt.Sprintf("Session %d case", sess.ID),
		StepsSnapshot: model.ToJSONArray(snapshot),
		Conclusion:    conclusion,
	}
	if err := svc.store.CreateCase(ctx, c); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}
	return OpResult{Op: op.Op, Status: "created"}
}

// ---- helpers ----

func (svc *Service) embed(ctx context.Context, it *model.Item) {
	if svc.embedder == nil || !slices.Contains(model.EmbeddedItemTypes, it.ItemType) {
		return
	}
	vecs, err := svc.embedder.Embed(ctx, []string{it.Title + "\n" + it.Summary})
	if err != nil || len(vecs) == 0 {
		slog.WarnContext(ctx, "embed item failed; it will be found by tags and text only", "error", err)
		return
	}
	v := pgvector.NewVector(vecs[0])
	it.Embedding = &v
}

func (svc *Service) recordHistory(ctx context.Context, sessionID, itemID int64, op string, before model.JSONB, after *model.Item) {
	err := svc.store.CreateItemHistory(ctx, &model.ItemHistory{
		ItemID:     itemID,
		SessionID:  sessionID,
		Op:         op,
		BeforeData: before,
		AfterData:  itemToJSON(after),
	})
	if err != nil {
		slog.WarnContext(ctx, "record item history failed; this change can't be reverted", "item_id", itemID, "error", err)
	}
}

func needsEvidenceResult(op string) OpResult {
	return OpResult{
		Op:      op,
		Status:  "needs_evidence",
		Message: op + " requires evidence_steps citing query steps from this session with result_class ok/no_data/wrong_mapping",
	}
}

// hasQueryEvidence reports whether any query step in the session observed something real.
func hasQueryEvidence(steps []model.Step) bool {
	for _, s := range steps {
		if s.StepType == "query" && evidenceClasses[s.ResultClass] {
			return true
		}
	}
	return false
}

// validEvidenceSteps reports whether at least one cited seq is a query step of this session with an evidence result.
func validEvidenceSteps(cited []int, steps []model.Step) bool {
	for _, seq := range cited {
		for _, s := range steps {
			if s.Seq == seq && s.StepType == "query" && evidenceClasses[s.ResultClass] {
				return true
			}
		}
	}
	return false
}

func itemToJSON(it *model.Item) model.JSONB {
	b, _ := json.Marshal(it)
	var m model.JSONB
	_ = json.Unmarshal(b, &m)
	return m
}
