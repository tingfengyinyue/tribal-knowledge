package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tingfengyinyue/tribal-knowledge/internal/bootstrap"
	"github.com/tingfengyinyue/tribal-knowledge/internal/session"
	"github.com/tingfengyinyue/tribal-knowledge/internal/store"
)

type MCPServer struct {
	svc   *session.Service
	store *store.Store
	srv   *server.MCPServer
}

func New(svc *session.Service, st *store.Store) *MCPServer {
	s := &MCPServer{svc: svc, store: st}
	s.srv = server.NewMCPServer(
		"tribal-knowledge",
		"1.0.0",
		server.WithToolCapabilities(true),
	)
	s.registerTools()
	return s
}

func (s *MCPServer) Server() *server.MCPServer {
	return s.srv
}

func prop(typ, desc string) any {
	return map[string]any{"type": typ, "description": desc}
}

func (s *MCPServer) registerTools() {
	s.srv.AddTool(mcp.Tool{
		Name:        "skill_open",
		Description: descOpen,
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"question":   prop("string", "The user's question"),
				"request_id": prop("string", "Client-generated idempotency key"),
				"client":     prop("object", "Client info with name, bootstrap_version, git_remote"),
				"max_items":  prop("integer", "Maximum items to return"),
			},
			Required: []string{"question"},
		},
	}, s.handleOpen)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_close",
		Description: descClose,
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session_id":        prop("integer", "Session ID"),
				"question_abstract": prop("string", "Short summary of the question"),
				"conclusion":        prop("object", "Conclusion with status, summary, root_causes, identifiers"),
			},
			Required: []string{"session_id", "conclusion"},
		},
	}, s.handleClose)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_search",
		Description: "Search the knowledge base for relevant notes.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session_id": prop("integer", "Session ID"),
				"query":      prop("string", "Search query"),
				"item_types": prop("array", "Filter by item types"),
				"services":   prop("array", "Filter by services"),
				"max_items":  prop("integer", "Max results"),
			},
			Required: []string{"session_id", "query"},
		},
	}, s.handleSearch)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_read",
		Description: "Read the full text of a knowledge item.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session_id": prop("integer", "Session ID"),
				"item_id":    prop("integer", "Item ID"),
				"section":    prop("string", "Section name for long items"),
			},
			Required: []string{"session_id", "item_id"},
		},
	}, s.handleRead)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_log_step",
		Description: descLogStep,
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session_id":    prop("integer", "Session ID"),
				"seq":           prop("integer", "Step sequence number"),
				"step_type":     prop("string", "env_check, plan, query, note"),
				"tool":          prop("string", "Tool used"),
				"command":       prop("string", "Command with credentials removed"),
				"resource":      prop("object", "Resource reference with node_key"),
				"result_class":  prop("string", "ok, no_data, wrong_mapping, ..."),
				"rows":          prop("integer", "Row count"),
				"summary":       prop("string", "Step summary"),
				"ids_found":     prop("object", "IDs discovered"),
				"used_item_ids": prop("array", "Item IDs referenced"),
			},
			Required: []string{"session_id", "seq", "step_type"},
		},
	}, s.handleLogStep)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_get_session",
		Description: "Get compressed session trace for reflection.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session_id": prop("integer", "Session ID"),
			},
			Required: []string{"session_id"},
		},
	}, s.handleGetSession)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_update",
		Description: descUpdate,
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"session_id":     prop("integer", "Session ID"),
				"update_id":      prop("string", "Client-generated idempotency key"),
				"user_confirmed": prop("boolean", "Whether user confirmed the update"),
				"feedback":       prop("array", "Item feedback"),
				"reflection":     prop("object", "Session reflection"),
				"operations":     prop("array", "Write-back operations"),
			},
			Required: []string{"session_id"},
		},
	}, s.handleUpdate)

	s.srv.AddTool(mcp.Tool{
		Name:        "skill_get_script",
		Description: "Get the bootstrap script. Compare bootstrap_version to check for updates.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"current_version": prop("string", "Your current bootstrap version"),
			},
		},
	}, s.handleGetScript)
}

// ---- Handlers ----

func (s *MCPServer) handleOpen(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in session.OpenInput
	if err := req.BindArguments(&in); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	out, err := s.svc.Open(ctx, in)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(out), nil
}

func (s *MCPServer) handleClose(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in session.CloseInput
	if err := req.BindArguments(&in); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	out, err := s.svc.Close(ctx, in)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(out), nil
}

func (s *MCPServer) handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	maxItems := req.GetInt("max_items", 30)

	ranked, err := s.svc.SearchItems(ctx, query, maxItems)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"items":   ranked,
		"notices": []string{},
	}), nil
}

func (s *MCPServer) handleRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemID := int64(req.GetInt("item_id", 0))
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return errResult("item not found"), nil
	}
	return jsonResult(map[string]any{
		"id":        item.ID,
		"item_type": item.ItemType,
		"title":     item.Title,
		"content":   item.Content,
		"tags":      item.Tags,
		"trust":     item.Trust,
	}), nil
}

func (s *MCPServer) handleLogStep(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in session.LogStepInput
	if err := req.BindArguments(&in); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	out, err := s.svc.LogStep(ctx, in)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(out), nil
}

func (s *MCPServer) handleGetSession(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := int64(req.GetInt("session_id", 0))
	out, err := s.svc.GetSession(ctx, sessionID)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(out), nil
}

func (s *MCPServer) handleUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in session.UpdateInput
	if err := req.BindArguments(&in); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	out, err := s.svc.Update(ctx, in)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(out), nil
}

func (s *MCPServer) handleGetScript(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	currentVersion := req.GetArguments()["current_version"]
	result := map[string]any{
		"version": bootstrap.Version,
		"script":  bootstrap.Script,
	}
	if cv, ok := currentVersion.(string); ok && cv == bootstrap.Version {
		result["up_to_date"] = true
	}
	return jsonResult(result), nil
}

// ---- helpers ----

func jsonResult(v any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: string(b),
		}},
	}
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf(`{"error":{"code":"INTERNAL","message":%q}}`, msg),
		}},
	}
}
