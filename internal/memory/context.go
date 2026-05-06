package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (s *Service) GetRelevantContext(ctx context.Context, in ContextInput) (string, []Memory, error) {
	if in.Limit <= 0 {
		in.Limit = 8
	}
	if in.Limit > 20 {
		in.Limit = 20
	}
	workspaceID, repoID, err := s.resolveWorkspaceRepo(ctx, in.WorkspaceID, in.RepoPath)
	if err != nil {
		return "", nil, err
	}
	query := strings.TrimSpace(in.Prompt + " " + strings.Join(in.FilePaths, " "))
	found, err := s.Search(ctx, SearchInput{
		Query:       query,
		WorkspaceID: workspaceID,
		RepoPath:    in.RepoPath,
		UserID:      in.UserID,
		Limit:       in.Limit,
	})
	if err != nil {
		return "", nil, err
	}
	selected := dedupeMemories(found)
	essential, err := s.essentialCoreMemories(ctx, repoID, in.UserID, in.Limit)
	if err != nil {
		return "", nil, err
	}
	selected = dedupeMemories(append(selected, essential...))
	neighbors, err := s.strongNeighbors(ctx, selected, in.Limit)
	if err != nil {
		return "", nil, err
	}
	selected = dedupeMemories(append(selected, neighbors...))
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Type == selected[j].Type {
			if selected[i].Importance == selected[j].Importance {
				return selected[i].CreatedAt.After(selected[j].CreatedAt)
			}
			return selected[i].Importance > selected[j].Importance
		}
		return typeRank(selected[i].Type) < typeRank(selected[j].Type)
	})
	if len(selected) > in.Limit+6 {
		selected = selected[:in.Limit+6]
	}
	notes, err := s.supersessionNotes(ctx, selected)
	if err != nil {
		return "", nil, err
	}
	return formatContextPatch(selected, notes), selected, nil
}

func (s *Service) essentialCoreMemories(ctx context.Context, repoID, userID string, limit int) ([]Memory, error) {
	if repoID == "" && userID == "" {
		return nil, nil
	}
	args := []any{}
	where := []string{`status = 'active'`}
	parts := []string{}
	if repoID != "" {
		parts = append(parts, `(repo_id = ? AND type IN (?, ?))`)
		args = append(args, repoID, TypeCoreConstraint, TypeCoreNegative)
	}
	if userID != "" {
		parts = append(parts, `(user_id = ? AND type = ?)`)
		args = append(args, userID, TypeCorePreference)
	}
	if len(parts) == 0 {
		return nil, nil
	}
	where = append(where, "("+strings.Join(parts, " OR ")+")")
	args = append(args, limit+5)
	rows, err := s.store.DB.QueryContext(ctx, `SELECT `+memorySelectColumns+` FROM memory_nodes WHERE `+strings.Join(where, " AND ")+` ORDER BY importance DESC, datetime(created_at) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (s *Service) strongNeighbors(ctx context.Context, selected []Memory, limit int) ([]Memory, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	out := []Memory{}
	for _, m := range selected {
		rows, err := s.store.DB.QueryContext(ctx, `
SELECT `+prefixMemoryColumns("n")+`
FROM memory_edges e
JOIN memory_nodes n ON (n.id = e.src_node_id OR n.id = e.dst_node_id)
WHERE (e.src_node_id = ? OR e.dst_node_id = ?)
  AND n.id <> ?
  AND n.status = 'active'
ORDER BY e.weight DESC, datetime(n.created_at) DESC
LIMIT 1`, m.ID, m.ID, m.ID)
		if err != nil {
			return nil, err
		}
		memories, scanErr := scanMemories(rows)
		_ = rows.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, memories...)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Service) supersessionNotes(ctx context.Context, selected []Memory) ([]string, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	notes := []string{}
	for _, m := range selected {
		rows, err := s.store.DB.QueryContext(ctx, `
SELECT new.title
FROM memory_edges e
JOIN memory_nodes old ON old.id = e.src_node_id
JOIN memory_nodes new ON new.id = e.dst_node_id
WHERE e.relation = 'superseded_by' AND e.dst_node_id = ? AND old.status = 'superseded'
ORDER BY datetime(e.created_at) DESC
LIMIT 2`, m.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var newTitle string
			if err := rows.Scan(&newTitle); err != nil {
				_ = rows.Close()
				return nil, err
			}
			notes = append(notes, fmt.Sprintf("%s supersedes an older memory.", newTitle))
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return notes, nil
}

func prefixMemoryColumns(prefix string) string {
	cols := []string{
		"id", "COALESCE(workspace_id, '')", "COALESCE(repo_id, '')", "COALESCE(user_id, '')",
		"COALESCE(session_id, '')", "type", "scope", "authority", "status", "title", "body",
		"COALESCE(tags, '[]')", "COALESCE(entities, '[]')", "COALESCE(file_paths, '[]')",
		"importance", "confidence", "COALESCE(superseded_by, '')", "COALESCE(created_by, '')",
		"created_at", "updated_at",
	}
	for i, col := range cols {
		if strings.Contains(col, "(") {
			cols[i] = strings.ReplaceAll(col, "(", "("+prefix+".")
			cols[i] = strings.ReplaceAll(cols[i], ", '", ", '")
			continue
		}
		cols[i] = prefix + "." + col
	}
	return strings.Join(cols, ", ")
}

func formatContextPatch(memories []Memory, supersessionNotes []string) string {
	var b strings.Builder
	b.WriteString("<coremem_context>\n")
	b.WriteString("Reason: retrieved from persistent DB memory for this repo/user/session.\n\n")
	writeSection(&b, "Relevant constraints", filterType(memories, TypeCoreConstraint), 220)
	writeSection(&b, "Relevant negative memories", filterType(memories, TypeCoreNegative), 220)
	writeSection(&b, "Relevant decisions", filterTypes(memories, TypeCoreDecision, TypeDerivedNote, TypeAgentResult), 260)
	writeSection(&b, "Relevant user preferences", filterType(memories, TypeCorePreference), 220)
	b.WriteString("Supersession notes:\n")
	if len(supersessionNotes) == 0 {
		b.WriteString("- None.\n\n")
	} else {
		for _, note := range supersessionNotes {
			b.WriteString("- ")
			b.WriteString(limitWords(note, 32))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	files := relevantFiles(memories)
	b.WriteString("Relevant files:\n")
	if len(files) == 0 {
		b.WriteString("- None recorded.\n")
	} else {
		for _, file := range files {
			b.WriteString("- ")
			b.WriteString(file)
			b.WriteString("\n")
		}
	}
	b.WriteString("</coremem_context>")
	return limitWordsPreserveLines(b.String(), 1200)
}

func writeSection(b *strings.Builder, name string, memories []Memory, bodyBudget int) {
	b.WriteString(name)
	b.WriteString(":\n")
	if len(memories) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	used := 0
	for _, m := range memories {
		item := m.Title + ": " + oneLine(m.Body)
		words := strings.Fields(item)
		if used+len(words) > bodyBudget && used > 0 {
			break
		}
		used += len(words)
		b.WriteString("- ")
		b.WriteString(limitWords(item, 60))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func filterType(memories []Memory, typ string) []Memory {
	return filterTypes(memories, typ)
}

func filterTypes(memories []Memory, types ...string) []Memory {
	set := map[string]bool{}
	for _, typ := range types {
		set[typ] = true
	}
	out := []Memory{}
	for _, m := range memories {
		if set[m.Type] {
			out = append(out, m)
		}
	}
	return out
}

func relevantFiles(memories []Memory) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range memories {
		for _, file := range m.FilePaths {
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			out = append(out, file)
			if len(out) >= 12 {
				return out
			}
		}
	}
	sort.Strings(out)
	return out
}

func dedupeMemories(in []Memory) []Memory {
	seen := map[string]bool{}
	out := make([]Memory, 0, len(in))
	for _, m := range in {
		if m.ID == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	return out
}

func typeRank(t string) int {
	switch t {
	case TypeCoreConstraint:
		return 0
	case TypeCoreNegative:
		return 1
	case TypeCoreDecision:
		return 2
	case TypeCorePreference:
		return 3
	default:
		return 4
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func limitWords(s string, max int) string {
	words := strings.Fields(s)
	if len(words) <= max {
		return s
	}
	return strings.Join(words[:max], " ") + "..."
}

func limitWordsPreserveLines(s string, max int) string {
	words := strings.Fields(s)
	if len(words) <= max {
		return s
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	used := 0
	for _, line := range lines {
		n := len(strings.Fields(line))
		if used+n > max {
			break
		}
		used += n
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n</coremem_context>"
}
