package hooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"coremem/internal/memory"
	"coremem/internal/parser"
)

type Server struct {
	svc *memory.Service
	mux *http.ServeMux
}

func NewServer(svc *memory.Service) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux()}
	s.mux.HandleFunc("/hooks/user-prompt-submit", s.handleUserPromptSubmit)
	s.mux.HandleFunc("/hooks/stop", s.handleStop)
	s.mux.HandleFunc("/hooks/session-start", s.handleSessionStart)
	s.mux.HandleFunc("/hooks/pre-compact", s.handlePreCompact)
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleUserPromptSubmit(w http.ResponseWriter, r *http.Request) {
	payload, raw, ok := readPayload(w, r)
	if !ok {
		return
	}
	prompt := firstString(payload, "prompt", "user_prompt", "message", "input")
	repoPath := firstString(payload, "cwd", "repo_path", "workspace_dir")
	workspaceID := firstString(payload, "workspace_id", "workspaceId")
	userID := firstString(payload, "user_id", "userId", "user")
	userID = fallbackQuery(r, userID, "user_id", "user")
	sessionID := firstString(payload, "session_id", "sessionId")
	repoID := s.resolveForEvent(r.Context(), workspaceID, repoPath, userID, sessionID, "user_prompt_submit")
	_ = s.svc.StoreEvent(r.Context(), sessionID, repoID, userID, "user_prompt_submit", string(raw))
	s.saveBlocks(r.Context(), prompt, memory.AuthorityUserTagged, workspaceID, repoPath, userID, sessionID)
	patch, _, _ := s.svc.GetRelevantContext(r.Context(), memory.ContextInput{
		WorkspaceID: workspaceID,
		RepoPath:    repoPath,
		UserID:      userID,
		Prompt:      prompt,
		Limit:       8,
	})
	writeHookOutput(w, r, "UserPromptSubmit", patch)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	payload, raw, ok := readPayload(w, r)
	if !ok {
		return
	}
	repoPath := firstString(payload, "cwd", "repo_path", "workspace_dir")
	workspaceID := firstString(payload, "workspace_id", "workspaceId")
	userID := firstString(payload, "user_id", "userId", "user")
	userID = fallbackQuery(r, userID, "user_id", "user")
	sessionID := firstString(payload, "session_id", "sessionId")
	repoID := s.resolveForEvent(r.Context(), workspaceID, repoPath, userID, sessionID, "stop")
	_ = s.svc.StoreEvent(r.Context(), sessionID, repoID, userID, "stop", string(raw))
	s.saveBlocks(r.Context(), strings.Join(allStrings(payload), "\n"), memory.AuthorityAgentTagged, workspaceID, repoPath, userID, sessionID)
	writeHookOutput(w, r, "Stop", "")
}

func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	payload, raw, ok := readPayload(w, r)
	if !ok {
		return
	}
	source := firstString(payload, "source", "session_source")
	repoPath := firstString(payload, "cwd", "repo_path", "workspace_dir")
	workspaceID := firstString(payload, "workspace_id", "workspaceId")
	userID := firstString(payload, "user_id", "userId", "user")
	userID = fallbackQuery(r, userID, "user_id", "user")
	sessionID := firstString(payload, "session_id", "sessionId")
	repoID := s.resolveForEvent(r.Context(), workspaceID, repoPath, userID, sessionID, source)
	_ = s.svc.StoreEvent(r.Context(), sessionID, repoID, userID, "session_start", string(raw))
	patch := ""
	switch strings.ToLower(source) {
	case "compact", "resume", "startup":
		patch, _, _ = s.svc.GetRelevantContext(r.Context(), memory.ContextInput{
			WorkspaceID: workspaceID,
			RepoPath:    repoPath,
			UserID:      userID,
			Prompt:      "recent high importance repo user memories",
			Limit:       8,
		})
	}
	writeHookOutput(w, r, "SessionStart", patch)
}

func (s *Server) handlePreCompact(w http.ResponseWriter, r *http.Request) {
	payload, raw, ok := readPayload(w, r)
	if !ok {
		return
	}
	repoPath := firstString(payload, "cwd", "repo_path", "workspace_dir")
	workspaceID := firstString(payload, "workspace_id", "workspaceId")
	userID := firstString(payload, "user_id", "userId", "user")
	userID = fallbackQuery(r, userID, "user_id", "user")
	sessionID := firstString(payload, "session_id", "sessionId")
	repoID := s.resolveForEvent(r.Context(), workspaceID, repoPath, userID, sessionID, "pre_compact")
	_ = s.svc.StoreEvent(r.Context(), sessionID, repoID, userID, "pre_compact", string(raw))
	instruction := "Before compaction, save important durable decisions, constraints, preferences, and negative memories with the MCP tool coremem_add. Do not store raw transcripts."
	writeHookOutput(w, r, "PreCompact", instruction)
}

func (s *Server) saveBlocks(ctx context.Context, text, authority, workspaceID, repoPath, userID, sessionID string) {
	blocks, err := parser.ParseBlocks(text)
	if err != nil {
		return
	}
	for _, block := range blocks {
		_, _ = s.svc.AddMemory(ctx, memory.AddInput{
			Type:        block.Type,
			Scope:       block.Scope,
			Title:       block.Title,
			Body:        block.Body,
			WorkspaceID: workspaceID,
			RepoPath:    repoPath,
			UserID:      userID,
			SessionID:   sessionID,
			Importance:  0.8,
			Confidence:  1,
			Authority:   authority,
			CreatedBy:   userID,
		})
	}
}

func (s *Server) resolveForEvent(ctx context.Context, workspaceID, repoPath, userID, sessionID, source string) string {
	var repoID string
	if repoPath != "" {
		if repo, err := s.svc.ResolveRepo(ctx, workspaceID, repoPath); err == nil {
			repoID = repo.ID
			workspaceID = repo.WorkspaceID
		}
	}
	if userID != "" {
		_ = s.svc.EnsureUser(ctx, userID)
	}
	_ = s.svc.EnsureSession(ctx, sessionID, workspaceID, repoID, userID, source)
	return repoID
}

func readPayload(w http.ResponseWriter, r *http.Request) (any, []byte, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, nil, false
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return nil, nil, false
	}
	var payload any
	if len(strings.TrimSpace(string(raw))) == 0 {
		payload = map[string]any{}
		raw = []byte(`{}`)
	} else if err := json.Unmarshal(raw, &payload); err != nil {
		payload = map[string]any{"raw": string(raw)}
	}
	return payload, raw, true
}

func writeHookOutput(w http.ResponseWriter, r *http.Request, name, contextPatch string) {
	if r.URL.Query().Get("plain") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(contextPatch))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"continue": true,
		"hookSpecificOutput": map[string]any{
			"hookEventName":     name,
			"additionalContext": contextPatch,
		},
	})
}

func fallbackQuery(r *http.Request, current string, keys ...string) string {
	if current != "" {
		return current
	}
	for _, key := range keys {
		if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func firstString(v any, keys ...string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(key)] = true
	}
	var walk func(any) string
	walk = func(node any) string {
		switch x := node.(type) {
		case map[string]any:
			for k, v := range x {
				if keySet[strings.ToLower(k)] {
					if s, ok := v.(string); ok {
						return s
					}
				}
			}
			for _, v := range x {
				if s := walk(v); s != "" {
					return s
				}
			}
		case []any:
			for _, v := range x {
				if s := walk(v); s != "" {
					return s
				}
			}
		}
		return ""
	}
	return walk(v)
}

func allStrings(v any) []string {
	var out []string
	var walk func(any)
	walk = func(node any) {
		switch x := node.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(v)
	return out
}
