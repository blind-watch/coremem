package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"coremem/internal/db"
	"coremem/internal/mcpserver"
	"coremem/internal/memory"
)

func TestToolsListExposesCorememTools(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "coremem.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var output bytes.Buffer
	if err := mcpserver.New(memory.NewService(store)).Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &resp); err != nil {
		t.Fatalf("bad response %q: %v", output.String(), err)
	}
	if !hasTool(resp.Result.Tools, "coremem_get_context") || !hasTool(resp.Result.Tools, "coremem_add") {
		t.Fatalf("expected coremem tools, got %+v", resp.Result.Tools)
	}
}

func TestToolCallAddUsesSnakeCaseArguments(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "coremem.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	req := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"coremem_add","arguments":{"type":"core_constraint","scope":"repo","title":"No in-memory queues","body":"Use persistent job state.","repo_path":"` + repo + `","user_id":"arjun","importance":0.9}}}` + "\n"
	var output bytes.Buffer
	if err := mcpserver.New(memory.NewService(store)).Serve(context.Background(), strings.NewReader(req), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"isError":false`) {
		t.Fatalf("tool call failed: %s", output.String())
	}
}

func hasTool(tools []struct {
	Name string `json:"name"`
}, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
