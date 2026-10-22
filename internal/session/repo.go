package session

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
)

var genericNames = map[string]bool{
	"api": true, "web": true, "app": true, "server": true, "client": true,
	"frontend": true, "backend": true, "service": true, "ui": true, "admin": true,
	"portal": true, "gateway": true, "proxy": true, "core": true, "common": true,
	"utils": true, "tools": true, "lib": true, "sdk": true, "cli": true,
	"test": true, "tests": true, "docs": true, "config": true, "scripts": true,
}

var repoURLPattern = regexp.MustCompile(`(?:git@[^:]+:|(?:https?|ssh)://[^/]+(?::\d+)?/)(.+?)(?:\.git)?/?$`)

func normalizeRepo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Remove credentials from URL
	raw = regexp.MustCompile(`://[^@]+@`).ReplaceAllString(raw, "://")

	m := repoURLPattern.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	path := strings.Trim(m[1], "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return path
}

func (svc *Service) repoContext(ctx context.Context, repo string) *RepoContext {
	node, err := svc.store.GetServiceByRepo(ctx, repo)
	if err != nil {
		return &RepoContext{
			Repo:   repo,
			Status: "unknown",
			Hint:   "This repository is not registered. Use register_repo in skill_update to register it.",
		}
	}
	return &RepoContext{
		Repo:   repo,
		Status: "known",
		Hint:   fmt.Sprintf("Mapped to %s (%s)", node.NodeKey, node.Name),
	}
}

func (svc *Service) registerRepo(ctx context.Context, op Operation) OpResult {
	if op.Repo == nil {
		return OpResult{Op: op.Op, Status: "invalid", Message: "repo field is required"}
	}

	repo := normalizeRepo(op.Repo.GitRemote)
	if repo == "" {
		return OpResult{Op: op.Op, Status: "rejected", Message: "invalid git remote"}
	}

	// Check generic name
	parts := strings.Split(repo, "/")
	repoName := parts[len(parts)-1]
	if genericNames[strings.ToLower(repoName)] {
		return OpResult{Op: op.Op, Status: "rejected", Message: fmt.Sprintf("repo name %q is too generic", repoName)}
	}

	// Check if already registered
	if existing, err := svc.store.GetServiceByRepo(ctx, repo); err == nil {
		return OpResult{Op: op.Op, Status: "exists", NodeKey: existing.NodeKey, Message: "already registered"}
	}

	// Check that the service name matches the repo
	serviceName := op.Repo.ServiceName
	if serviceName == "" {
		serviceName = repoName
	}

	if !repoMatches(repo, []string{serviceName}) {
		return OpResult{Op: op.Op, Status: "rejected", Message: "service name does not match repo"}
	}

	nodeKey := "service:" + compactName(serviceName)
	attrs := model.JSONB{"repo": repo}
	if op.Repo.Description != "" {
		attrs["description"] = op.Repo.Description
	}
	if len(op.Repo.Aliases) > 0 {
		attrs["aliases"] = op.Repo.Aliases
	}

	node := &model.Node{
		NodeKey:    nodeKey,
		NodeType:   "service",
		Name:       serviceName,
		Attributes: attrs,
	}
	if err := svc.store.UpsertNode(ctx, node); err != nil {
		return OpResult{Op: op.Op, Status: "error", Message: err.Error()}
	}

	return OpResult{Op: op.Op, Status: "created", NodeKey: nodeKey}
}

func repoMatches(repo string, names []string) bool {
	parts := strings.Split(repo, "/")
	repoName := strings.ToLower(parts[len(parts)-1])

	for _, name := range names {
		name = strings.ToLower(name)
		compact := compactName(name)
		compactRepo := compactName(repoName)

		if compact == compactRepo {
			return true
		}
		if strings.Contains(compactRepo, compact) || strings.Contains(compact, compactRepo) {
			return true
		}
	}
	return false
}

func compactName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func filterAliases(aliases []string, selfKey string, owners map[string]string) (kept, dropped []string) {
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" || genericNames[strings.ToLower(a)] {
			dropped = append(dropped, a)
			continue
		}
		if ownerKey, ok := owners[strings.ToLower(a)]; ok && ownerKey != selfKey {
			dropped = append(dropped, a)
			continue
		}
		kept = append(kept, a)
	}
	return
}
