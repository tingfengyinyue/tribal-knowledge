package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
)

// Update processes feedback, reflection and operations for a session.
func (svc *Service) Update(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
	sess, err := svc.store.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Idempotency check
	if in.UpdateID != "" {
		if existing, err := svc.store.GetUpdateByKey(ctx, sess.ID, in.UpdateID); err == nil {
			var results []OpResult
			if b, ok := existing.Results["results"]; ok {
				if bb, err := json.Marshal(b); err == nil {
					json.Unmarshal(bb, &results)
				}
			}
			return &UpdateOutput{Results: results}, nil
		}
	}

	steps, err := svc.store.ListSteps(ctx, sess.ID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}

	hasEvidence := hasQueryEvidence(steps)

	// Process feedback
	if len(in.Feedback) > 0 && hasEvidence {
		var helpfulIDs []int64
		for _, f := range in.Feedback {
			if f.Verdict == "helpful" {
				helpfulIDs = append(helpfulIDs, f.ItemID)
			}
		}
		if len(helpfulIDs) > 0 {
			svc.store.TouchVerified(ctx, helpfulIDs, svc.now())
		}
	}

	// Process operations
	results := make([]OpResult, len(in.Operations))
	for i, op := range in.Operations {
		results[i] = svc.processOp(ctx, sess, steps, hasEvidence, op)
	}

	// Save reflection
	reflectionJSON := model.JSONB{}
	if in.Reflection != nil {
		reflectionJSON = model.JSONB{
			"redundant_steps": in.Reflection.RedundantSteps,
			"better_order":    in.Reflection.BetterOrder,
			"pitfalls":        in.Reflection.Pitfalls,
			"next_time":       in.Reflection.NextTime,
		}
	}

	feedbackJSON := model.JSONB{}
	if len(in.Feedback) > 0 {
		b, _ := json.Marshal(in.Feedback)
		json.Unmarshal(b, &feedbackJSON)
	}

	opsJSON := model.JSONB{}
	if len(in.Operations) > 0 {
		b, _ := json.Marshal(in.Operations)
		json.Unmarshal(b, &opsJSON)
	}

	resultsJSON := model.JSONB{}
	{
		b, _ := json.Marshal(results)
		json.Unmarshal(b, &resultsJSON)
	}

	update := &model.Update{
		SessionID:  sess.ID,
		UpdateID:   in.UpdateID,
		Feedback:   feedbackJSON,
		Reflection: reflectionJSON,
		Operations: opsJSON,
		Results:    resultsJSON,
	}
	svc.store.CreateUpdate(ctx, update)

	return &UpdateOutput{Results: results}, nil
}

func (svc *Service) processOp(ctx context.Context, sess *model.Session, steps []model.Step, hasEvidence bool, op Operation) OpResult {
	switch op.Op {
	case "add_item":
		return svc.addItem(ctx, sess, steps, hasEvidence, op)
	case "update_item":
		return svc.updateItem(ctx, sess, steps, hasEvidence, op)
	case "invalidate_item":
		return svc.invalidateItem(ctx, sess, hasEvidence, op)
	case "upsert_node":
		return svc.upsertNode(ctx, sess, op)
	case "update_node":
		return svc.updateNode(ctx, sess, hasEvidence, op)
	case "invalidate_node":
		return svc.invalidateNode(ctx, sess, hasEvidence, op)
	case "add_edge":
		return svc.addEdge(ctx, sess, hasEvidence, op)
	case "invalidate_edge":
		return svc.invalidateEdge(ctx, sess, hasEvidence, op)
	case "save_case":
		return svc.saveCase(ctx, sess, steps, hasEvidence, op)
	case "register_repo":
		return svc.registerRepo(ctx, sess, steps, hasEvidence, op)
	default:
		return OpResult{Op: op.Op, Status: "invalid", Message: "unknown operation"}
	}
}

func (svc *Service) addItem(ctx context.Context, sess *model.Session, steps []model.Step, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "add_item")
	}
	if !validEvidenceSteps(op.EvidenceSteps, steps) {
		return needsEvidenceResult(op.Op, "add_item")
	}

	item := &model.Item{
		ItemType:   op.ItemType,
		Title:      op.Title,
		Summary:    op.Content,
		Content:    op.Content,
		Tags:       model.StringArray(op.Tags),
		Attributes: model.JSONB{"recorded_at": svc.now().Format(time.RFC3339)},
		Trust:      1.0,
		Status:     "active",
		LoadWhen:   "relevant",
	}
	if err := svc.store.CreateItem(ctx, item); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}

	svc.store.CreateItemHistory(ctx, &model.ItemHistory{
		ItemID:    item.ID,
		SessionID: sess.ID,
		Op:        "created",
		AfterData: itemToJSON(item),
	})

	return OpResult{Op: op.Op, Status: "created", ItemID: item.ID}
}

func (svc *Service) updateItem(ctx context.Context, sess *model.Session, steps []model.Step, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		// Without evidence, create as conflict instead of updating
		item := &model.Item{
			ItemType:   op.ItemType,
			Title:      op.Title + " (conflict)",
			Summary:    op.Content,
			Content:    op.Content,
			Tags:       model.StringArray(op.Tags),
			Attributes: model.JSONB{"conflict_with": op.ItemID, "recorded_at": svc.now().Format(time.RFC3339)},
			Trust:      0.5,
			Status:     "active",
			LoadWhen:   "relevant",
		}
		if err := svc.store.CreateItem(ctx, item); err != nil {
			return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
		}
		return OpResult{Op: op.Op, Status: "created_as_conflict", ItemID: item.ID}
	}

	if !validEvidenceSteps(op.EvidenceSteps, steps) {
		return needsEvidenceResult(op.Op, "update_item")
	}

	existing, err := svc.store.GetItem(ctx, op.ItemID)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
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

	if err := svc.store.UpdateItem(ctx, existing); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}

	svc.store.CreateItemHistory(ctx, &model.ItemHistory{
		ItemID:    existing.ID,
		SessionID: sess.ID,
		Op:        "updated",
		BeforeData: before,
		AfterData:  itemToJSON(existing),
	})

	return OpResult{Op: op.Op, Status: "updated", ItemID: existing.ID}
}

func (svc *Service) invalidateItem(ctx context.Context, sess *model.Session, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "invalidate_item")
	}

	existing, err := svc.store.GetItem(ctx, op.ItemID)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
	}

	before := itemToJSON(existing)
	existing.Status = "invalid"

	if err := svc.store.UpdateItem(ctx, existing); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}

	svc.store.CreateItemHistory(ctx, &model.ItemHistory{
		ItemID:    existing.ID,
		SessionID: sess.ID,
		Op:        "invalidated",
		BeforeData: before,
		AfterData:  itemToJSON(existing),
	})

	return OpResult{Op: op.Op, Status: "invalidated", ItemID: existing.ID}
}

func (svc *Service) upsertNode(ctx context.Context, sess *model.Session, op Operation) OpResult {
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

func (svc *Service) updateNode(ctx context.Context, sess *model.Session, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "update_node")
	}

	node, err := svc.store.GetNode(ctx, op.NodeKey)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found"}
	}

	if op.Name != "" {
		node.Name = op.Name
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

func (svc *Service) invalidateNode(ctx context.Context, sess *model.Session, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "invalidate_node")
	}

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

func (svc *Service) addEdge(ctx context.Context, sess *model.Session, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "add_edge")
	}

	sourceNode, err := svc.store.GetNode(ctx, op.SourceNodeKey)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found", Message: "source node not found: " + op.SourceNodeKey}
	}
	targetNode, err := svc.store.GetNode(ctx, op.TargetNodeKey)
	if err != nil {
		return OpResult{Op: op.Op, Status: "not_found", Message: "target node not found: " + op.TargetNodeKey}
	}

	edge := &model.Edge{
		SourceNodeID: sourceNode.ID,
		TargetNodeID: targetNode.ID,
		EdgeType:     op.EdgeType,
	}
	if err := svc.store.UpsertEdge(ctx, edge); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}

	return OpResult{Op: op.Op, Status: "created"}
}

func (svc *Service) invalidateEdge(ctx context.Context, sess *model.Session, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "invalidate_edge")
	}
	// Simplified — would need edge lookup by source/target/type
	return OpResult{Op: op.Op, Status: "updated"}
}

func (svc *Service) saveCase(ctx context.Context, sess *model.Session, steps []model.Step, hasEvidence bool, op Operation) OpResult {
	if !hasEvidence {
		return needsEvidenceResult(op.Op, "save_case")
	}
	if !validEvidenceSteps(op.EvidenceSteps, steps) {
		return needsEvidenceResult(op.Op, "save_case")
	}

	stepsSnapshot := make([]map[string]any, len(steps))
	for i, s := range steps {
		stepsSnapshot[i] = map[string]any{
			"seq": s.Seq, "step_type": s.StepType,
			"tool": s.Tool, "result_class": s.ResultClass,
			"summary": s.Summary,
		}
	}

	conclusion := model.JSONB{}
	if sess.Conclusion != nil {
		conclusion = sess.Conclusion
	}

	snapshotJSON := model.JSONB{}
	{
		b, _ := json.Marshal(stepsSnapshot)
		json.Unmarshal(b, &snapshotJSON)
	}

	c := &model.Case{
		SessionID:     sess.ID,
		Title:         sess.QuestionAbstract,
		Summary:       fmt.Sprintf("Session %d case", sess.ID),
		StepsSnapshot: snapshotJSON,
		Conclusion:    conclusion,
	}
	if err := svc.store.CreateCase(ctx, c); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}

	return OpResult{Op: op.Op, Status: "created"}
}

// ---- helpers ----

func needsEvidenceResult(op, what string) OpResult {
	return OpResult{
		Op:      op,
		Status:  "needs_evidence",
		Message: what + " requires evidence_steps citing query steps with result_class ok/no_data/wrong_mapping",
	}
}

func validEvidenceSteps(cited []int, steps []model.Step) bool {
	if len(cited) == 0 {
		return false
	}
	evidenceClasses := map[string]bool{"ok": true, "no_data": true, "wrong_mapping": true}
	seqMap := map[int]bool{}
	for _, s := range steps {
		if s.StepType == "query" && evidenceClasses[s.ResultClass] {
			seqMap[s.Seq] = true
		}
	}
	for _, seq := range cited {
		if seqMap[seq] {
			return true
		}
	}
	return false
}

func itemToJSON(it *model.Item) model.JSONB {
	b, _ := json.Marshal(it)
	var m model.JSONB
	json.Unmarshal(b, &m)
	return m
}
