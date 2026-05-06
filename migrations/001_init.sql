CREATE TABLE IF NOT EXISTS schema_migrations (
  name TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS repos (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL UNIQUE,
  git_remote TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT,
  repo_id TEXT,
  user_id TEXT,
  source TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY (repo_id) REFERENCES repos(id),
  FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS memory_nodes (
  id TEXT PRIMARY KEY,
  workspace_id TEXT,
  repo_id TEXT,
  user_id TEXT,
  session_id TEXT,
  type TEXT NOT NULL,
  scope TEXT NOT NULL,
  authority TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  entities TEXT NOT NULL DEFAULT '[]',
  file_paths TEXT NOT NULL DEFAULT '[]',
  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 1.0,
  superseded_by TEXT,
  created_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY (repo_id) REFERENCES repos(id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (session_id) REFERENCES sessions(id),
  FOREIGN KEY (superseded_by) REFERENCES memory_nodes(id)
);

-- Future Postgres + pgvector migration point:
-- ALTER TABLE memory_nodes ADD COLUMN embedding vector(1536);

CREATE TABLE IF NOT EXISTS memory_edges (
  id TEXT PRIMARY KEY,
  src_node_id TEXT NOT NULL,
  dst_node_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  weight REAL NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (src_node_id) REFERENCES memory_nodes(id),
  FOREIGN KEY (dst_node_id) REFERENCES memory_nodes(id)
);

CREATE TABLE IF NOT EXISTS memory_events (
  id TEXT PRIMARY KEY,
  session_id TEXT,
  repo_id TEXT,
  user_id TEXT,
  event_type TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (session_id) REFERENCES sessions(id),
  FOREIGN KEY (repo_id) REFERENCES repos(id),
  FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_repos_root_path ON repos(root_path);
CREATE INDEX IF NOT EXISTS idx_memory_nodes_lookup ON memory_nodes(status, repo_id, user_id, type);
CREATE INDEX IF NOT EXISTS idx_memory_nodes_created ON memory_nodes(created_at);
CREATE INDEX IF NOT EXISTS idx_memory_edges_src ON memory_edges(src_node_id);
CREATE INDEX IF NOT EXISTS idx_memory_edges_dst ON memory_edges(dst_node_id);
CREATE INDEX IF NOT EXISTS idx_memory_events_created ON memory_events(created_at);
