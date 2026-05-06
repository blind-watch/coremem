package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"coremem/internal/db"
	"coremem/internal/search"
)

const DefaultWorkspaceID = "default"

type Service struct {
	store *db.Store
	now   func() time.Time
}

func NewService(store *db.Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) AddMemory(ctx context.Context, in AddInput) (Memory, error) {
	in = normalizeAdd(in)
	if err := validateAddInput(in); err != nil {
		return Memory{}, err
	}
	workspaceID, repoID, err := s.resolveWorkspaceRepo(ctx, in.WorkspaceID, in.RepoPath)
	if err != nil {
		return Memory{}, err
	}
	if in.UserID != "" {
		if err := s.EnsureUser(ctx, in.UserID); err != nil {
			return Memory{}, err
		}
	}
	if in.SessionID != "" {
		if err := s.EnsureSession(ctx, in.SessionID, workspaceID, repoID, in.UserID, ""); err != nil {
			return Memory{}, err
		}
	}
	id, err := newID("mem")
	if err != nil {
		return Memory{}, err
	}
	now := s.now()
	tags, _ := json.Marshal(cleanStrings(in.Tags))
	entities, _ := json.Marshal(cleanStrings(in.Entities))
	files, _ := json.Marshal(cleanStrings(in.FilePaths))
	_, err = s.store.DB.ExecContext(ctx, `
INSERT INTO memory_nodes (
  id, workspace_id, repo_id, user_id, session_id, type, scope, authority, status,
  title, body, tags, entities, file_paths, importance, confidence, created_by,
  created_at, updated_at
) VALUES (?, nullif(?, ''), nullif(?, ''), nullif(?, ''), nullif(?, ''), ?, ?, ?, ?,
  ?, ?, ?, ?, ?, ?, ?, nullif(?, ''), ?, ?)`,
		id, workspaceID, repoID, in.UserID, in.SessionID, in.Type, in.Scope, in.Authority, StatusActive,
		strings.TrimSpace(in.Title), strings.TrimSpace(in.Body), string(tags), string(entities), string(files),
		in.Importance, in.Confidence, in.CreatedBy, formatTime(now), formatTime(now))
	if err != nil {
		return Memory{}, err
	}
	return s.GetMemory(ctx, id)
}

func normalizeAdd(in AddInput) AddInput {
	in.Type = strings.TrimSpace(in.Type)
	if in.Scope == "" {
		in.Scope = ScopeRepo
	}
	in.Scope = strings.TrimSpace(in.Scope)
	if in.Scope == ScopeWorkspace && in.WorkspaceID == "" {
		in.WorkspaceID = DefaultWorkspaceID
	}
	if in.Authority == "" {
		in.Authority = AuthorityUserTagged
	}
	if in.Importance == 0 {
		in.Importance = 0.5
	}
	if in.Confidence == 0 {
		in.Confidence = 1
	}
	return in
}

func (s *Service) GetMemory(ctx context.Context, id string) (Memory, error) {
	row := s.store.DB.QueryRowContext(ctx, `SELECT `+memorySelectColumns+` FROM memory_nodes WHERE id = ?`, id)
	return scanMemory(row)
}

func (s *Service) Search(ctx context.Context, in SearchInput) ([]Memory, error) {
	if in.Limit <= 0 {
		in.Limit = 10
	}
	if in.Limit > 50 {
		in.Limit = 50
	}
	workspaceID, repoID, err := s.resolveWorkspaceRepo(ctx, in.WorkspaceID, in.RepoPath)
	if err != nil {
		return nil, err
	}
	terms := search.Terms(in.Query)
	args := []any{}
	where := []string{}
	if in.IncludeSuperseded {
		where = append(where, `status <> 'archived'`)
	} else {
		where = append(where, `status = 'active'`)
	}
	if repoID != "" {
		where = append(where, `(repo_id = ? OR repo_id IS NULL)`)
		args = append(args, repoID)
	} else if workspaceID != "" {
		where = append(where, `(workspace_id = ? OR workspace_id IS NULL)`)
		args = append(args, workspaceID)
	}
	if in.UserID != "" {
		where = append(where, `(scope <> 'user' OR user_id = ?)`)
		args = append(args, in.UserID)
	} else {
		where = append(where, `scope <> 'user'`)
	}
	if len(terms) > 0 {
		termParts := make([]string, 0, len(terms))
		for _, term := range terms {
			like := "%" + strings.ToLower(term) + "%"
			termParts = append(termParts, `(lower(title) LIKE ? OR lower(body) LIKE ? OR lower(tags) LIKE ? OR lower(entities) LIKE ? OR lower(file_paths) LIKE ?)`)
			args = append(args, like, like, like, like, like)
		}
		where = append(where, "("+strings.Join(termParts, " OR ")+")")
	}
	query := `SELECT ` + memorySelectColumns + ` FROM memory_nodes WHERE ` + strings.Join(where, " AND ")
	rows, err := s.store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memories, err := scanMemories(rows)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for i := range memories {
		memories[i].Score = search.Score(search.Candidate{
			Title:      memories[i].Title,
			Body:       memories[i].Body,
			Tags:       memories[i].Tags,
			Entities:   memories[i].Entities,
			FilePaths:  memories[i].FilePaths,
			Type:       memories[i].Type,
			Status:     memories[i].Status,
			RepoID:     memories[i].RepoID,
			UserID:     memories[i].UserID,
			Importance: memories[i].Importance,
			CreatedAt:  memories[i].CreatedAt,
		}, in.Query, terms, repoID, in.UserID, now)
	}
	sort.SliceStable(memories, func(i, j int) bool {
		if memories[i].Score == memories[j].Score {
			return memories[i].CreatedAt.After(memories[j].CreatedAt)
		}
		return memories[i].Score > memories[j].Score
	})
	if len(memories) > in.Limit {
		memories = memories[:in.Limit]
	}
	return memories, nil
}

func (s *Service) Recent(ctx context.Context, repoPath, userID string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	_, repoID, err := s.resolveWorkspaceRepo(ctx, "", repoPath)
	if err != nil {
		return nil, err
	}
	args := []any{}
	where := []string{`status = 'active'`}
	if repoID != "" {
		where = append(where, `(repo_id = ? OR repo_id IS NULL)`)
		args = append(args, repoID)
	}
	if userID != "" {
		where = append(where, `(scope <> 'user' OR user_id = ?)`)
		args = append(args, userID)
	} else {
		where = append(where, `scope <> 'user'`)
	}
	args = append(args, limit)
	rows, err := s.store.DB.QueryContext(ctx, `SELECT `+memorySelectColumns+` FROM memory_nodes WHERE `+strings.Join(where, " AND ")+` ORDER BY datetime(created_at) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (s *Service) Supersede(ctx context.Context, in SupersedeInput) (Memory, error) {
	if strings.TrimSpace(in.OldMemoryID) == "" {
		return Memory{}, errors.New("old_memory_id is required")
	}
	old, err := s.GetMemory(ctx, in.OldMemoryID)
	if err != nil {
		return Memory{}, err
	}
	repoPath := in.RepoPath
	workspaceID := old.WorkspaceID
	if repoPath == "" && old.RepoID != "" {
		repo, err := s.GetRepo(ctx, old.RepoID)
		if err == nil {
			repoPath = repo.RootPath
			workspaceID = repo.WorkspaceID
		}
	}
	userID := in.UserID
	if userID == "" {
		userID = old.UserID
	}
	newMem, err := s.AddMemory(ctx, AddInput{
		Type:        in.NewType,
		Scope:       old.Scope,
		Title:       in.NewTitle,
		Body:        in.NewBody,
		Tags:        old.Tags,
		Entities:    old.Entities,
		FilePaths:   old.FilePaths,
		WorkspaceID: workspaceID,
		RepoPath:    repoPath,
		UserID:      userID,
		SessionID:   old.SessionID,
		Importance:  old.Importance,
		Confidence:  old.Confidence,
		Authority:   AuthorityUserTagged,
		CreatedBy:   userID,
	})
	if err != nil {
		return Memory{}, err
	}
	now := formatTime(s.now())
	_, err = s.store.DB.ExecContext(ctx, `UPDATE memory_nodes SET status = ?, superseded_by = ?, updated_at = ? WHERE id = ?`, StatusSuperseded, newMem.ID, now, old.ID)
	if err != nil {
		return Memory{}, err
	}
	edge, err := s.Link(ctx, LinkInput{SrcNodeID: old.ID, DstNodeID: newMem.ID, Relation: "superseded_by", Weight: 1})
	if err != nil {
		return Memory{}, err
	}
	_ = edge
	if strings.TrimSpace(in.Reason) != "" {
		payload, _ := json.Marshal(map[string]string{
			"old_memory_id": old.ID,
			"new_memory_id": newMem.ID,
			"reason":        in.Reason,
		})
		_ = s.StoreEvent(ctx, "", newMem.RepoID, userID, "memory_superseded", string(payload))
	}
	return newMem, nil
}

func (s *Service) Link(ctx context.Context, in LinkInput) (string, error) {
	if err := validateLinkInput(in); err != nil {
		return "", err
	}
	id, err := newID("edge")
	if err != nil {
		return "", err
	}
	_, err = s.store.DB.ExecContext(ctx, `INSERT INTO memory_edges (id, src_node_id, dst_node_id, relation, weight, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, in.SrcNodeID, in.DstNodeID, in.Relation, in.Weight, formatTime(s.now()))
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) StoreEvent(ctx context.Context, sessionID, repoID, userID, eventType, payload string) error {
	if strings.TrimSpace(eventType) == "" {
		return errors.New("event type is required")
	}
	id, err := newID("evt")
	if err != nil {
		return err
	}
	_, err = s.store.DB.ExecContext(ctx, `INSERT INTO memory_events (id, session_id, repo_id, user_id, event_type, payload, created_at) VALUES (?, nullif(?, ''), nullif(?, ''), nullif(?, ''), ?, ?, ?)`,
		id, sessionID, repoID, userID, eventType, payload, formatTime(s.now()))
	return err
}

func (s *Service) EventCount(ctx context.Context) (int, error) {
	var count int
	err := s.store.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_events`).Scan(&count)
	return count, err
}

func (s *Service) EnsureWorkspace(ctx context.Context, workspaceID string) (string, error) {
	if strings.TrimSpace(workspaceID) == "" {
		workspaceID = DefaultWorkspaceID
	}
	_, err := s.store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO workspaces (id, name, created_at) VALUES (?, ?, ?)`,
		workspaceID, workspaceID, formatTime(s.now()))
	return workspaceID, err
}

func (s *Service) EnsureUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	_, err := s.store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO users (id, name, created_at) VALUES (?, ?, ?)`,
		userID, userID, formatTime(s.now()))
	return err
}

func (s *Service) ResolveRepo(ctx context.Context, workspaceID, repoPath string) (Repo, error) {
	if strings.TrimSpace(repoPath) == "" {
		return Repo{}, nil
	}
	root, err := normalizeRepoPath(repoPath)
	if err != nil {
		return Repo{}, err
	}
	var repo Repo
	row := s.store.DB.QueryRowContext(ctx, `SELECT id, workspace_id, name, root_path, COALESCE(git_remote, ''), created_at FROM repos WHERE root_path = ?`, root)
	if err := row.Scan(&repo.ID, &repo.WorkspaceID, &repo.Name, &repo.RootPath, &repo.GitRemote, scanTimePtr(&repo.CreatedAt)); err == nil {
		return repo, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Repo{}, err
	}
	if workspaceID == "" {
		workspaceID = DefaultWorkspaceID
	}
	if _, err := s.EnsureWorkspace(ctx, workspaceID); err != nil {
		return Repo{}, err
	}
	id, err := newID("repo")
	if err != nil {
		return Repo{}, err
	}
	now := s.now()
	repo = Repo{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        filepath.Base(root),
		RootPath:    root,
		CreatedAt:   now,
	}
	_, err = s.store.DB.ExecContext(ctx, `INSERT INTO repos (id, workspace_id, name, root_path, git_remote, created_at) VALUES (?, ?, ?, ?, null, ?)`,
		repo.ID, repo.WorkspaceID, repo.Name, repo.RootPath, formatTime(now))
	if err != nil {
		return Repo{}, err
	}
	return repo, nil
}

func (s *Service) GetRepo(ctx context.Context, repoID string) (Repo, error) {
	var repo Repo
	row := s.store.DB.QueryRowContext(ctx, `SELECT id, workspace_id, name, root_path, COALESCE(git_remote, ''), created_at FROM repos WHERE id = ?`, repoID)
	err := row.Scan(&repo.ID, &repo.WorkspaceID, &repo.Name, &repo.RootPath, &repo.GitRemote, scanTimePtr(&repo.CreatedAt))
	return repo, err
}

func (s *Service) EnsureSession(ctx context.Context, sessionID, workspaceID, repoID, userID, source string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	now := formatTime(s.now())
	_, err := s.store.DB.ExecContext(ctx, `
INSERT INTO sessions (id, workspace_id, repo_id, user_id, source, created_at, updated_at)
VALUES (?, nullif(?, ''), nullif(?, ''), nullif(?, ''), nullif(?, ''), ?, ?)
ON CONFLICT(id) DO UPDATE SET
  workspace_id = COALESCE(excluded.workspace_id, sessions.workspace_id),
  repo_id = COALESCE(excluded.repo_id, sessions.repo_id),
  user_id = COALESCE(excluded.user_id, sessions.user_id),
  source = COALESCE(excluded.source, sessions.source),
  updated_at = excluded.updated_at`,
		sessionID, workspaceID, repoID, userID, source, now, now)
	return err
}

func (s *Service) resolveWorkspaceRepo(ctx context.Context, workspaceID, repoPath string) (string, string, error) {
	if repoPath != "" {
		repo, err := s.ResolveRepo(ctx, workspaceID, repoPath)
		if err != nil {
			return "", "", err
		}
		return repo.WorkspaceID, repo.ID, nil
	}
	if workspaceID != "" {
		workspaceID, err := s.EnsureWorkspace(ctx, workspaceID)
		return workspaceID, "", err
	}
	return "", "", nil
}

func normalizeRepoPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

const memorySelectColumns = `
id, COALESCE(workspace_id, ''), COALESCE(repo_id, ''), COALESCE(user_id, ''),
COALESCE(session_id, ''), type, scope, authority, status, title, body,
COALESCE(tags, '[]'), COALESCE(entities, '[]'), COALESCE(file_paths, '[]'),
importance, confidence, COALESCE(superseded_by, ''), COALESCE(created_by, ''),
created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMemory(row rowScanner) (Memory, error) {
	var m Memory
	var tags, entities, files string
	var createdAt, updatedAt string
	err := row.Scan(
		&m.ID, &m.WorkspaceID, &m.RepoID, &m.UserID, &m.SessionID, &m.Type, &m.Scope,
		&m.Authority, &m.Status, &m.Title, &m.Body, &tags, &entities, &files, &m.Importance,
		&m.Confidence, &m.SupersededBy, &m.CreatedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		return Memory{}, err
	}
	m.Tags = decodeStringArray(tags)
	m.Entities = decodeStringArray(entities)
	m.FilePaths = decodeStringArray(files)
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return m, nil
}

func scanMemories(rows *sql.Rows) ([]Memory, error) {
	var out []Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func decodeStringArray(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, raw)
	return t
}

func scanTimePtr(dst *time.Time) any {
	return scannerFunc(func(src any) error {
		switch v := src.(type) {
		case string:
			*dst = parseTime(v)
		case []byte:
			*dst = parseTime(string(v))
		case time.Time:
			*dst = v
		case nil:
			*dst = time.Time{}
		default:
			return fmt.Errorf("unsupported time type %T", src)
		}
		return nil
	})
}

type scannerFunc func(src any) error

func (f scannerFunc) Scan(src any) error {
	return f(src)
}
