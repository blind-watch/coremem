package hooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"coremem/internal/db"
	"coremem/internal/hooks"
	"coremem/internal/memory"
)

func TestUserPromptSubmitStoresEventAndReturnsContext(t *testing.T) {
	ctx := context.Background()
	store, svc := newHookTestService(t)
	defer store.Close()
	repo := filepath.Join(t.TempDir(), "repo")
	if _, err := svc.AddMemory(ctx, memory.AddInput{
		Type:       memory.TypeCoreConstraint,
		Scope:      memory.ScopeRepo,
		Title:      "No in-memory queues",
		Body:       "Do not use in-memory queues for job processing.",
		RepoPath:   repo,
		UserID:     "arjun",
		Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"prompt":"Implement async job processing","cwd":"` + repo + `","user_id":"arjun","session_id":"s1"}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/user-prompt-submit", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	hooks.NewServer(svc).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Continue           bool `json:"continue"`
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Continue || !strings.Contains(resp.HookSpecificOutput.AdditionalContext, "No in-memory queues") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	count, err := svc.EventCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1", count)
	}
}

func TestUserPromptSubmitUsesUserIDQueryFallback(t *testing.T) {
	ctx := context.Background()
	store, svc := newHookTestService(t)
	defer store.Close()
	repo := filepath.Join(t.TempDir(), "repo")
	if _, err := svc.AddMemory(ctx, memory.AddInput{
		Type:       memory.TypeCorePreference,
		Scope:      memory.ScopeUser,
		Title:      "Arjun Go style",
		Body:       "Prefer explicit error handling.",
		RepoPath:   repo,
		UserID:     "arjun",
		Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"prompt":"Add tests","cwd":"` + repo + `","session_id":"s1"}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/user-prompt-submit?plain=1&user_id=arjun", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	hooks.NewServer(svc).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Arjun Go style") {
		t.Fatalf("query user_id preference missing: %s", rec.Body.String())
	}
}

func TestStopParsesCorememBlock(t *testing.T) {
	ctx := context.Background()
	store, svc := newHookTestService(t)
	defer store.Close()
	repo := filepath.Join(t.TempDir(), "repo")
	payload := `{"cwd":"` + repo + `","user_id":"arjun","assistant":"[coremem:type=core_negative scope=repo title=\"No FSx hot path\"]\nDo not use FSx for hot path storage.\n[/coremem]"}`
	req := httptest.NewRequest(http.MethodPost, "/hooks/stop", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	hooks.NewServer(svc).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	results, err := svc.Search(ctx, memory.SearchInput{Query: "FSx", RepoPath: repo, UserID: "arjun", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !containsMemoryTitle(results, "No FSx hot path") {
		t.Fatalf("parsed memory missing: %+v", results)
	}
}

func newHookTestService(t *testing.T) (*db.Store, *memory.Service) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "coremem.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store, memory.NewService(store)
}

func containsMemoryTitle(memories []memory.Memory, title string) bool {
	for _, m := range memories {
		if m.Title == title {
			return true
		}
	}
	return false
}
