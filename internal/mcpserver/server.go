package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"coremem/internal/memory"
)

type Server struct {
	svc *memory.Service
}

func New(svc *memory.Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		resp, ok := s.handle(ctx, []byte(line))
		if !ok {
			continue
		}
		if _, err := writer.Write(resp); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return scanner.Err()
}

type request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handle(ctx context.Context, raw []byte) ([]byte, bool) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return marshalResp(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}), true
	}
	if req.ID == nil {
		return nil, false
	}
	result, err := s.dispatch(ctx, req.Method, req.Params)
	if err != nil {
		return marshalResp(response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: err.Error()}}), true
	}
	return marshalResp(response{JSONRPC: "2.0", ID: req.ID, Result: result}), true
}

func marshalResp(resp response) []byte {
	b, err := json.Marshal(resp)
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal error"}}`)
	}
	return b
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2024-11-05"
		}
		return map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "coremem",
				"version": "0.1.0",
			},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		return s.callTool(ctx, params)
	default:
		return nil, fmt.Errorf("unsupported method %q", method)
	}
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, error) {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	switch p.Name {
	case "coremem_add":
		var in memory.AddInput
		if err := json.Unmarshal(p.Arguments, &in); err != nil {
			return nil, err
		}
		mem, err := s.svc.AddMemory(ctx, in)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolJSON(map[string]string{"id": mem.ID}), nil
	case "coremem_search":
		var in memory.SearchInput
		if err := json.Unmarshal(p.Arguments, &in); err != nil {
			return nil, err
		}
		memories, err := s.svc.Search(ctx, in)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolJSON(memories), nil
	case "coremem_get_context":
		var in memory.ContextInput
		if err := json.Unmarshal(p.Arguments, &in); err != nil {
			return nil, err
		}
		patch, _, err := s.svc.GetRelevantContext(ctx, in)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolText(patch), nil
	case "coremem_supersede":
		var in memory.SupersedeInput
		if err := json.Unmarshal(p.Arguments, &in); err != nil {
			return nil, err
		}
		mem, err := s.svc.Supersede(ctx, in)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolJSON(map[string]string{"id": mem.ID}), nil
	case "coremem_link":
		var in memory.LinkInput
		if err := json.Unmarshal(p.Arguments, &in); err != nil {
			return nil, err
		}
		id, err := s.svc.Link(ctx, in)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolJSON(map[string]string{"id": id}), nil
	case "coremem_recent":
		var in struct {
			RepoPath string `json:"repo_path"`
			UserID   string `json:"user_id"`
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(p.Arguments, &in); err != nil {
			return nil, err
		}
		memories, err := s.svc.Recent(ctx, in.RepoPath, in.UserID, in.Limit)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolJSON(memories), nil
	default:
		return toolError("unknown tool " + p.Name), nil
	}
}

func toolText(text string) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": false,
	}
}

func toolJSON(v any) map[string]any {
	b, _ := json.MarshalIndent(v, "", "  ")
	return toolText(string(b))
}

func toolError(text string) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": true,
	}
}

func toolDefinitions() []map[string]any {
	return []map[string]any{
		tool("coremem_add", "Store an explicit durable memory node.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":         enum([]string{memory.TypeCoreDecision, memory.TypeCoreConstraint, memory.TypeCoreNegative, memory.TypeCorePreference, memory.TypeDerivedNote, memory.TypeAgentResult}),
				"scope":        enum([]string{memory.ScopeGlobal, memory.ScopeWorkspace, memory.ScopeRepo, memory.ScopeUser, memory.ScopeSession}),
				"title":        str(),
				"body":         str(),
				"tags":         arr(),
				"entities":     arr(),
				"file_paths":   arr(),
				"workspace_id": str(),
				"repo_path":    str(),
				"user_id":      str(),
				"session_id":   str(),
				"importance":   num(),
			},
			"required": []string{"type", "scope", "title", "body"},
		}),
		tool("coremem_search", "Search ranked memories.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":              str(),
				"workspace_id":       str(),
				"repo_path":          str(),
				"user_id":            str(),
				"limit":              integer(),
				"include_superseded": map[string]string{"type": "boolean"},
			},
			"required": []string{"query"},
		}),
		tool("coremem_get_context", "Return a compact repo/user memory context patch.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":       str(),
				"workspace_id": str(),
				"repo_path":    str(),
				"user_id":      str(),
				"file_paths":   arr(),
				"limit":        integer(),
			},
			"required": []string{"prompt"},
		}),
		tool("coremem_supersede", "Create a newer memory and mark an older one superseded.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"old_memory_id": str(),
				"new_type":      enum([]string{memory.TypeCoreDecision, memory.TypeCoreConstraint, memory.TypeCoreNegative, memory.TypeCorePreference, memory.TypeDerivedNote, memory.TypeAgentResult}),
				"new_title":     str(),
				"new_body":      str(),
				"reason":        str(),
				"repo_path":     str(),
				"user_id":       str(),
			},
			"required": []string{"old_memory_id", "new_type", "new_title", "new_body", "reason"},
		}),
		tool("coremem_link", "Link two memories.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"src_node_id": str(),
				"dst_node_id": str(),
				"relation":    enum([]string{"supports", "contradicts", "supersedes", "related_to", "applies_to", "blocks", "depends_on"}),
				"weight":      num(),
			},
			"required": []string{"src_node_id", "dst_node_id", "relation", "weight"},
		}),
		tool("coremem_recent", "List recent active memories.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo_path": str(),
				"user_id":   str(),
				"limit":     integer(),
			},
		}),
	}
}

func tool(name, description string, schema map[string]any) map[string]any {
	return map[string]any{"name": name, "description": description, "inputSchema": schema}
}

func str() map[string]string {
	return map[string]string{"type": "string"}
}

func num() map[string]string {
	return map[string]string{"type": "number"}
}

func integer() map[string]string {
	return map[string]string{"type": "integer"}
}

func arr() map[string]any {
	return map[string]any{"type": "array", "items": str()}
}

func enum(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
