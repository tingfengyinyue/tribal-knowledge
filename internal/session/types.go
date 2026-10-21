package session

// ---- Open ----

type OpenInput struct {
	Question  string      `json:"question"`
	RequestID string      `json:"request_id,omitempty"`
	Client    ClientInfo  `json:"client"`
	MaxItems  int         `json:"max_items,omitempty"`
}

type ClientInfo struct {
	Name             string `json:"name"`
	BootstrapVersion string `json:"bootstrap_version"`
	GitRemote        string `json:"git_remote"`
}

type OpenOutput struct {
	SessionID        int64          `json:"session_id"`
	BootstrapVersion string         `json:"bootstrap_version"`
	Instruction      InstructionSet `json:"instruction"`
	Parsed           ParsedQuestion `json:"parsed"`
	Graph            GraphView      `json:"graph"`
	Items            []ItemView     `json:"items"`
	Notices          []string       `json:"notices"`
	RepoContext      *RepoContext   `json:"repo_context,omitempty"`
}

type InstructionSet struct {
	Data []InstructionItem `json:"data"`
}

type InstructionItem struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type ParsedQuestion struct {
	Services    []ServiceRef          `json:"services"`
	Env         string                `json:"env"`
	Identifiers map[string][]string   `json:"identifiers"`
	ErrorCodes  []string              `json:"error_codes"`
}

type ServiceRef struct {
	NodeKey string `json:"node_key"`
}

type GraphView struct {
	Services  []NodeView `json:"services"`
	Resources []NodeView `json:"resources"`
	Neighbors []EdgeView `json:"neighbors"`
}

type NodeView struct {
	ID       int64             `json:"id"`
	NodeKey  string            `json:"node_key"`
	Name     string            `json:"name"`
	NodeType string            `json:"node_type"`
	Attrs    map[string]any    `json:"attributes,omitempty"`
}

type EdgeView struct {
	SourceKey string `json:"source_key"`
	TargetKey string `json:"target_key"`
	EdgeType  string `json:"edge_type"`
}

type ItemView struct {
	ID           int64   `json:"id"`
	ItemType     string  `json:"item_type"`
	Title        string  `json:"title"`
	Summary      string  `json:"summary"`
	Brief        string  `json:"brief,omitempty"`
	Score        float64 `json:"score"`
	Trust        float32 `json:"trust"`
	LastVerified string  `json:"last_verified_at,omitempty"`
	Stale        bool    `json:"stale,omitempty"`
}

type RepoContext struct {
	Repo   string `json:"repo"`
	Status string `json:"status"` // known, unknown
	Hint   string `json:"hint,omitempty"`
}

// ---- Close ----

type CloseInput struct {
	SessionID        int64      `json:"session_id"`
	QuestionAbstract string     `json:"question_abstract"`
	Conclusion       Conclusion `json:"conclusion"`
}

type Conclusion struct {
	Status      string              `json:"status"` // confirmed, likely, not_reproduced, blocked, answered
	Summary     string              `json:"summary"`
	RootCauses  []string            `json:"root_causes"`
	Identifiers map[string][]string `json:"identifiers"`
}

type CloseOutput struct {
	Status   string         `json:"status"`
	Warnings []CloseWarning `json:"warnings,omitempty"`
}

type CloseWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ---- LogStep ----

type LogStepInput struct {
	SessionID   int64              `json:"session_id"`
	Seq         int                `json:"seq"`
	StepType    string             `json:"step_type"`
	Tool        string             `json:"tool"`
	Command     string             `json:"command"`
	Resource    *ResourceRef       `json:"resource,omitempty"`
	ResultClass string             `json:"result_class"`
	Rows        int                `json:"rows"`
	Summary     string             `json:"summary"`
	IDsFound    map[string][]string `json:"ids_found"`
	UsedItemIDs []int64            `json:"used_item_ids"`
}

type ResourceRef struct {
	NodeKey string `json:"node_key"`
}

type LogStepOutput struct {
	Hints []string `json:"hints,omitempty"`
}

// ---- Update ----

type UpdateInput struct {
	SessionID    int64        `json:"session_id"`
	UpdateID     string       `json:"update_id"`
	UserConfirmed bool        `json:"user_confirmed"`
	Feedback     []Feedback   `json:"feedback"`
	Reflection   *Reflection  `json:"reflection"`
	Operations   []Operation  `json:"operations"`
}

type Feedback struct {
	ItemID  int64  `json:"item_id"`
	Verdict string `json:"verdict"` // helpful, not_helpful, outdated
}

type Reflection struct {
	RedundantSteps []string `json:"redundant_steps"`
	BetterOrder    []string `json:"better_order"`
	Pitfalls       []string `json:"pitfalls"`
	NextTime       []string `json:"next_time"`
}

type Operation struct {
	Op            string   `json:"op"`
	ItemType      string   `json:"item_type,omitempty"`
	Title         string   `json:"title,omitempty"`
	Content       string   `json:"content,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	ItemID        int64    `json:"item_id,omitempty"`
	SourceNodeKey string   `json:"source_node_key,omitempty"`
	TargetNodeKey string   `json:"target_node_key,omitempty"`
	EdgeType      string   `json:"edge_type,omitempty"`
	NodeKey       string   `json:"node_key,omitempty"`
	NodeType      string   `json:"node_type,omitempty"`
	Name          string   `json:"name,omitempty"`
	Attributes    map[string]any `json:"attributes,omitempty"`
	EvidenceSteps []int    `json:"evidence_steps,omitempty"`
	OnSimilar     string   `json:"on_similar,omitempty"`
	MergeInto     int64    `json:"merge_into,omitempty"`
	Repo          *RepoRegistration `json:"repo,omitempty"`
}

type RepoRegistration struct {
	GitRemote   string   `json:"git_remote"`
	ServiceName string   `json:"service_name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
}

type UpdateOutput struct {
	Results []OpResult `json:"results"`
}

type OpResult struct {
	Op      string `json:"op"`
	Status  string `json:"status"` // created, updated, merged, invalidated, needs_evidence, rejected, ...
	ItemID  int64  `json:"item_id,omitempty"`
	NodeKey string `json:"node_key,omitempty"`
	Message string `json:"message,omitempty"`
}

// ---- GetSession ----

type GetSessionOutput struct {
	SessionID int64      `json:"session_id"`
	Question  string     `json:"question"`
	Steps     []StepView `json:"steps"`
}

type StepView struct {
	Seq         int    `json:"seq"`
	StepType    string `json:"step_type"`
	Tool        string `json:"tool,omitempty"`
	ResourceKey string `json:"resource_key,omitempty"`
	ResultClass string `json:"result_class,omitempty"`
	Summary     string `json:"summary"`
}
