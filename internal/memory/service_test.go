package memory_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"coremem/internal/db"
	"coremem/internal/memory"
)

func newTestService(t *testing.T) (*db.Store, *memory.Service) {
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

func TestAddAndSearchMemory(t *testing.T) {
	ctx := context.Background()
	store, svc := newTestService(t)
	defer store.Close()
	repo := filepath.Join(t.TempDir(), "repo")
	mem, err := svc.AddMemory(ctx, memory.AddInput{
		Type:       memory.TypeCoreConstraint,
		Scope:      memory.ScopeRepo,
		Title:      "No in-memory queues",
		Body:       "Use persistent job state instead of in-memory queues.",
		Tags:       []string{"queue"},
		RepoPath:   repo,
		UserID:     "arjun",
		Importance: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mem.ID == "" || mem.Status != memory.StatusActive {
		t.Fatalf("unexpected memory: %+v", mem)
	}
	results, err := svc.Search(ctx, memory.SearchInput{Query: "queue", RepoPath: repo, UserID: "arjun", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Title != "No in-memory queues" {
		t.Fatalf("search did not return memory: %+v", results)
	}
}

func TestSupersedeExcludesOldByDefault(t *testing.T) {
	ctx := context.Background()
	store, svc := newTestService(t)
	defer store.Close()
	repo := filepath.Join(t.TempDir(), "repo")
	old, err := svc.AddMemory(ctx, memory.AddInput{
		Type:       memory.TypeCoreDecision,
		Scope:      memory.ScopeRepo,
		Title:      "Use Redis queue",
		Body:       "Use Redis for async work.",
		RepoPath:   repo,
		Importance: 0.8,
	})
	if err != nil {
		t.Fatal(err)
	}
	newMem, err := svc.Supersede(ctx, memory.SupersedeInput{
		OldMemoryID: old.ID,
		NewType:     memory.TypeCoreDecision,
		NewTitle:    "Use persistent job table",
		NewBody:     "Use a SQLite-backed job table for async work.",
		Reason:      "local-first POC",
		RepoPath:    repo,
	})
	if err != nil {
		t.Fatal(err)
	}
	refetched, err := svc.GetMemory(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refetched.Status != memory.StatusSuperseded || refetched.SupersededBy != newMem.ID {
		t.Fatalf("old memory not superseded: %+v", refetched)
	}
	active, err := svc.Search(ctx, memory.SearchInput{Query: "Redis", RepoPath: repo, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range active {
		if m.ID == old.ID {
			t.Fatalf("superseded memory returned by default: %+v", active)
		}
	}
	withSuperseded, err := svc.Search(ctx, memory.SearchInput{Query: "Redis", RepoPath: repo, Limit: 10, IncludeSuperseded: true})
	if err != nil {
		t.Fatal(err)
	}
	if !containsTitle(withSuperseded, "Use Redis queue") {
		t.Fatalf("superseded memory missing when requested: %+v", withSuperseded)
	}
}

func TestContextRetrievalPrioritizesRepoUserCoreAndActive(t *testing.T) {
	ctx := context.Background()
	store, svc := newTestService(t)
	defer store.Close()
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	mustAdd := func(in memory.AddInput) memory.Memory {
		t.Helper()
		mem, err := svc.AddMemory(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return mem
	}
	mustAdd(memory.AddInput{
		Type:       memory.TypeCoreConstraint,
		Scope:      memory.ScopeRepo,
		Title:      "No in-memory queues",
		Body:       "Do not use in-memory queues for job processing because workers run across multiple pods.",
		RepoPath:   repoA,
		Importance: 0.95,
	})
	mustAdd(memory.AddInput{
		Type:       memory.TypeCoreConstraint,
		Scope:      memory.ScopeRepo,
		Title:      "Other repo queue rule",
		Body:       "Other repo may use an in-memory queue.",
		RepoPath:   repoB,
		Importance: 1,
	})
	mustAdd(memory.AddInput{
		Type:       memory.TypeCoreConstraint,
		Scope:      memory.ScopeRepo,
		Title:      "Idempotent handlers",
		Body:       "Job handlers must be idempotent because retries can duplicate execution.",
		RepoPath:   repoA,
		Importance: 0.9,
	})
	mustAdd(memory.AddInput{
		Type:       memory.TypeCoreNegative,
		Scope:      memory.ScopeRepo,
		Title:      "No new dependencies",
		Body:       "Do not add external dependencies unless explicitly approved.",
		RepoPath:   repoA,
		Importance: 0.85,
	})
	mustAdd(memory.AddInput{
		Type:       memory.TypeCorePreference,
		Scope:      memory.ScopeUser,
		Title:      "Arjun Go style",
		Body:       "Prefer explicit error handling, small functions, and table-driven tests.",
		RepoPath:   repoA,
		UserID:     "arjun",
		Importance: 0.9,
	})
	mustAdd(memory.AddInput{
		Type:       memory.TypeCorePreference,
		Scope:      memory.ScopeUser,
		Title:      "Meena Go style",
		Body:       "Prefer composable helpers and concise abstractions.",
		RepoPath:   repoA,
		UserID:     "meena",
		Importance: 0.9,
	})
	old := mustAdd(memory.AddInput{
		Type:       memory.TypeCoreConstraint,
		Scope:      memory.ScopeRepo,
		Title:      "Old queue decision",
		Body:       "Old queue guidance should not appear.",
		RepoPath:   repoA,
		Importance: 1,
	})
	if _, err := svc.Supersede(ctx, memory.SupersedeInput{
		OldMemoryID: old.ID,
		NewType:     memory.TypeCoreConstraint,
		NewTitle:    "New queue decision",
		NewBody:     "Use durable job state for queue work.",
		Reason:      "replaced old queue decision",
		RepoPath:    repoA,
	}); err != nil {
		t.Fatal(err)
	}
	patch, selected, err := svc.GetRelevantContext(ctx, memory.ContextInput{
		RepoPath:  repoA,
		UserID:    "arjun",
		Prompt:    "Implement async job processing",
		FilePaths: []string{"internal/jobs/jobs.go"},
		Limit:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"No in-memory queues", "Idempotent handlers", "No new dependencies", "Arjun Go style"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("context missing %q:\n%s", want, patch)
		}
	}
	for _, unwanted := range []string{"Other repo queue rule", "Meena Go style", "Old queue decision"} {
		if strings.Contains(patch, unwanted) {
			t.Fatalf("context included %q:\n%s", unwanted, patch)
		}
	}
	for _, m := range selected {
		if m.Status != memory.StatusActive {
			t.Fatalf("selected non-active memory: %+v", m)
		}
	}
}

func containsTitle(memories []memory.Memory, title string) bool {
	for _, m := range memories {
		if m.Title == title {
			return true
		}
	}
	return false
}
