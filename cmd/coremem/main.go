package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"coremem/internal/config"
	"coremem/internal/db"
	"coremem/internal/hooks"
	"coremem/internal/mcpserver"
	"coremem/internal/memory"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "coremem:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	ctx := context.Background()
	switch args[0] {
	case "migrate":
		store, err := openStore(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		fmt.Println("migrated", mustDBPath())
		return nil
	case "mcp":
		store, svc, err := openService(ctx)
		if err != nil {
			return err
		}
		defer store.Close()
		return mcpserver.New(svc).Serve(ctx, os.Stdin, os.Stdout)
	case "http":
		return runHTTP(ctx, args[1:])
	case "add":
		return runAdd(ctx, args[1:])
	case "search":
		return runSearch(ctx, args[1:])
	case "context":
		return runContext(ctx, args[1:])
	case "demo":
		return runDemo(ctx)
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func openStore(ctx context.Context) (*db.Store, error) {
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	store, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func openService(ctx context.Context) (*db.Store, *memory.Service, error) {
	store, err := openStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	return store, memory.NewService(store), nil
}

func runHTTP(ctx context.Context, args []string) error {
	parsed := parseArgs(args)
	addr := parsed.value("addr", "127.0.0.1:8765")
	store, svc, err := openService(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	server := &http.Server{
		Addr:    addr,
		Handler: hooks.NewServer(svc).Handler(),
	}
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintln(os.Stderr, "coremem http listening on", addr)
		errCh <- server.ListenAndServe()
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		fmt.Fprintln(os.Stderr, "coremem http shutting down:", sig)
		return server.Shutdown(context.Background())
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runAdd(ctx context.Context, args []string) error {
	parsed := parseArgs(args)
	store, svc, err := openService(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	mem, err := svc.AddMemory(ctx, memory.AddInput{
		Type:        parsed.value("type", ""),
		Scope:       parsed.value("scope", memory.ScopeRepo),
		Title:       parsed.value("title", ""),
		Body:        parsed.value("body", ""),
		Tags:        splitCSV(parsed.value("tags", "")),
		Entities:    splitCSV(parsed.value("entities", "")),
		FilePaths:   splitCSV(parsed.value("file-paths", "")),
		WorkspaceID: parsed.value("workspace-id", ""),
		RepoPath:    parsed.value("repo-path", ""),
		UserID:      parsed.value("user-id", ""),
		SessionID:   parsed.value("session-id", ""),
		Importance:  parsed.float("importance", 0.5),
		Confidence:  1,
		Authority:   memory.AuthorityUserTagged,
		CreatedBy:   parsed.value("user-id", ""),
	})
	if err != nil {
		return err
	}
	fmt.Println(mem.ID)
	return nil
}

func runSearch(ctx context.Context, args []string) error {
	parsed := parseArgs(args)
	query := parsed.value("query", "")
	if query == "" && len(parsed.positionals) > 0 {
		query = parsed.positionals[0]
	}
	store, svc, err := openService(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	memories, err := svc.Search(ctx, memory.SearchInput{
		Query:             query,
		WorkspaceID:       parsed.value("workspace-id", ""),
		RepoPath:          parsed.value("repo-path", ""),
		UserID:            parsed.value("user-id", ""),
		Limit:             parsed.int("limit", 10),
		IncludeSuperseded: parsed.bool("include-superseded"),
	})
	if err != nil {
		return err
	}
	for _, m := range memories {
		fmt.Printf("%.2f\t%s\t%s\t%s\t%s\n", m.Score, m.ID, m.Type, m.Scope, m.Title)
	}
	return nil
}

func runContext(ctx context.Context, args []string) error {
	parsed := parseArgs(args)
	prompt := parsed.value("prompt", "")
	if prompt == "" && len(parsed.positionals) > 0 {
		prompt = strings.Join(parsed.positionals, " ")
	}
	store, svc, err := openService(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	patch, _, err := svc.GetRelevantContext(ctx, memory.ContextInput{
		WorkspaceID: parsed.value("workspace-id", ""),
		RepoPath:    parsed.value("repo-path", ""),
		UserID:      parsed.value("user-id", ""),
		Prompt:      prompt,
		FilePaths:   splitCSV(parsed.value("file-paths", "")),
		Limit:       parsed.int("limit", 8),
	})
	if err != nil {
		return err
	}
	fmt.Println(patch)
	return nil
}

func runDemo(ctx context.Context) error {
	store, svc, err := openService(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	repoPath, _ := os.Getwd()
	seed := []memory.AddInput{
		{
			Type:        memory.TypeCoreConstraint,
			Scope:       memory.ScopeRepo,
			Title:       "No in-memory queues",
			Body:        "Do not use in-memory queues for job processing because workers run across multiple pods. Use persistent job state instead.",
			Tags:        []string{"jobs", "queue", "persistence"},
			WorkspaceID: "demo",
			RepoPath:    repoPath,
			Importance:  0.95,
		},
		{
			Type:        memory.TypeCoreConstraint,
			Scope:       memory.ScopeRepo,
			Title:       "Idempotent handlers",
			Body:        "Job handlers must be idempotent because retries can duplicate execution.",
			Tags:        []string{"jobs", "retries", "idempotency"},
			WorkspaceID: "demo",
			RepoPath:    repoPath,
			Importance:  0.9,
		},
		{
			Type:        memory.TypeCoreNegative,
			Scope:       memory.ScopeRepo,
			Title:       "No new dependencies",
			Body:        "Do not add external dependencies unless explicitly approved.",
			Tags:        []string{"dependencies"},
			WorkspaceID: "demo",
			RepoPath:    repoPath,
			Importance:  0.85,
		},
		{
			Type:        memory.TypeCorePreference,
			Scope:       memory.ScopeUser,
			Title:       "Arjun Go style",
			Body:        "Prefer explicit error handling, small functions, simple interfaces, and table-driven tests. Avoid clever abstractions.",
			Tags:        []string{"go", "style"},
			WorkspaceID: "demo",
			RepoPath:    repoPath,
			UserID:      "arjun",
			Importance:  0.9,
		},
		{
			Type:        memory.TypeCorePreference,
			Scope:       memory.ScopeUser,
			Title:       "Meena Go style",
			Body:        "Prefer composable helpers, reusable validation functions, and concise service-layer abstractions.",
			Tags:        []string{"go", "style"},
			WorkspaceID: "demo",
			RepoPath:    repoPath,
			UserID:      "meena",
			Importance:  0.9,
		},
	}
	added := 0
	for _, item := range seed {
		item.Authority = memory.AuthorityUserTagged
		item.Confidence = 1
		if exists, err := memoryExists(ctx, svc, item); err != nil {
			return err
		} else if exists {
			continue
		}
		if _, err := svc.AddMemory(ctx, item); err != nil {
			return err
		}
		added++
	}
	fmt.Printf("demo seeded %d new memories for repo %s\n", added, repoPath)
	return nil
}

func memoryExists(ctx context.Context, svc *memory.Service, item memory.AddInput) (bool, error) {
	memories, err := svc.Search(ctx, memory.SearchInput{
		Query:       item.Title,
		WorkspaceID: item.WorkspaceID,
		RepoPath:    item.RepoPath,
		UserID:      item.UserID,
		Limit:       20,
	})
	if err != nil {
		return false, err
	}
	for _, m := range memories {
		if m.Type == item.Type && m.Title == item.Title && m.UserID == item.UserID {
			return true, nil
		}
	}
	return false, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  coremem migrate
  coremem mcp
  coremem http --addr 127.0.0.1:8765
  coremem add --type core_constraint --scope repo --title "..." --body "..." --repo-path . --user-id arjun
  coremem search "query" --repo-path . --user-id arjun
  coremem context --prompt "..." --repo-path . --user-id arjun
  coremem demo`)
}

func mustDBPath() string {
	path, err := config.DBPath()
	if err != nil {
		return "<unknown>"
	}
	return path
}

type parsedArgs struct {
	positionals []string
	values      map[string]string
	bools       map[string]bool
}

func parseArgs(args []string) parsedArgs {
	p := parsedArgs{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name, val, ok := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			if ok {
				p.values[name] = val
				if val == "true" {
					p.bools[name] = true
				}
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				p.values[name] = args[i+1]
				if args[i+1] == "true" {
					p.bools[name] = true
				}
				i++
			} else {
				p.bools[name] = true
				p.values[name] = "true"
			}
			continue
		}
		p.positionals = append(p.positionals, arg)
	}
	return p
}

func (p parsedArgs) value(key, def string) string {
	if v, ok := p.values[key]; ok {
		return v
	}
	return def
}

func (p parsedArgs) bool(key string) bool {
	if v, ok := p.values[key]; ok {
		b, _ := strconv.ParseBool(v)
		return b
	}
	return p.bools[key]
}

func (p parsedArgs) int(key string, def int) int {
	if v, ok := p.values[key]; ok {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func (p parsedArgs) float(key string, def float64) float64 {
	if v, ok := p.values[key]; ok {
		n, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
