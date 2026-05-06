package memory

import "time"

const (
	TypeCoreDecision   = "core_decision"
	TypeCoreConstraint = "core_constraint"
	TypeCoreNegative   = "core_negative"
	TypeCorePreference = "core_preference"
	TypeDerivedNote    = "derived_note"
	TypeAgentResult    = "agent_result"

	ScopeGlobal    = "global"
	ScopeWorkspace = "workspace"
	ScopeRepo      = "repo"
	ScopeUser      = "user"
	ScopeSession   = "session"

	AuthorityUserTagged     = "user_tagged"
	AuthorityAgentTagged    = "agent_tagged"
	AuthoritySystemObserved = "system_observed"

	StatusActive     = "active"
	StatusSuperseded = "superseded"
	StatusArchived   = "archived"
)

type Memory struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspace_id,omitempty"`
	RepoID       string    `json:"repo_id,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	Type         string    `json:"type"`
	Scope        string    `json:"scope"`
	Authority    string    `json:"authority"`
	Status       string    `json:"status"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	Tags         []string  `json:"tags,omitempty"`
	Entities     []string  `json:"entities,omitempty"`
	FilePaths    []string  `json:"file_paths,omitempty"`
	Importance   float64   `json:"importance"`
	Confidence   float64   `json:"confidence"`
	SupersededBy string    `json:"superseded_by,omitempty"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Score        float64   `json:"score,omitempty"`
}

type Repo struct {
	ID          string
	WorkspaceID string
	Name        string
	RootPath    string
	GitRemote   string
	CreatedAt   time.Time
}

type Session struct {
	ID          string
	WorkspaceID string
	RepoID      string
	UserID      string
	Source      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type AddInput struct {
	Type        string   `json:"type"`
	Scope       string   `json:"scope"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Tags        []string `json:"tags"`
	Entities    []string `json:"entities"`
	FilePaths   []string `json:"file_paths"`
	WorkspaceID string   `json:"workspace_id"`
	RepoPath    string   `json:"repo_path"`
	UserID      string   `json:"user_id"`
	SessionID   string   `json:"session_id"`
	Importance  float64  `json:"importance"`
	Confidence  float64  `json:"confidence"`
	Authority   string   `json:"authority"`
	CreatedBy   string   `json:"created_by"`
}

type SearchInput struct {
	Query             string `json:"query"`
	WorkspaceID       string `json:"workspace_id"`
	RepoPath          string `json:"repo_path"`
	UserID            string `json:"user_id"`
	Limit             int    `json:"limit"`
	IncludeSuperseded bool   `json:"include_superseded"`
}

type ContextInput struct {
	WorkspaceID string   `json:"workspace_id"`
	RepoPath    string   `json:"repo_path"`
	UserID      string   `json:"user_id"`
	Prompt      string   `json:"prompt"`
	FilePaths   []string `json:"file_paths"`
	Limit       int      `json:"limit"`
}

type SupersedeInput struct {
	OldMemoryID string `json:"old_memory_id"`
	NewType     string `json:"new_type"`
	NewTitle    string `json:"new_title"`
	NewBody     string `json:"new_body"`
	Reason      string `json:"reason"`
	RepoPath    string `json:"repo_path"`
	UserID      string `json:"user_id"`
}

type LinkInput struct {
	SrcNodeID string  `json:"src_node_id"`
	DstNodeID string  `json:"dst_node_id"`
	Relation  string  `json:"relation"`
	Weight    float64 `json:"weight"`
}
