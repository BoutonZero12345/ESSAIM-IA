package message

// PacketType defines the allowed message types in the Inter-Agent Protocol (IAP).
type PacketType string

// Commands (Parent -> Child)
const (
	CmdTask PacketType = "CMD_TASK"
	CmdKill PacketType = "CMD_KILL"
	CmdWipe PacketType = "CMD_WIPE"
)

// Requests (Worker -> Postier)
const (
	ReqSpawn  PacketType = "REQ_SPAWN"
	ReqResume PacketType = "REQ_RESUME"
	ReqInfo   PacketType = "REQ_INFO"
	ReqBudget PacketType = "REQ_BUDGET"
)

// Reports (Worker -> Postier)
const (
	RptDone     PacketType = "RPT_DONE"
	RptFail     PacketType = "RPT_FAIL"
	RptProgress PacketType = "RPT_PROGRESS"
)

// Header contains routing metadata for a Packet.
type Header struct {
	ID        string     `bson:"id" json:"id"`
	Timestamp int64      `bson:"timestamp" json:"timestamp"`
	From      string     `bson:"from" json:"from"`
	To        string     `bson:"to" json:"to"`
	Type      PacketType `bson:"type" json:"type"`
}

// Meta contains telemetry data for a Packet.
type Meta struct {
	TokensUsed int   `bson:"tokens_used" json:"tokens_used"`
	LatencyMs  int64 `bson:"latency_ms" json:"latency_ms"`
}

// Packet is the fundamental communication unit between agents (The Envelope).
// Agents NEVER exchange raw text — only strict JSON Packets.
type Packet struct {
	Head Header                 `bson:"head" json:"head"`
	Body map[string]interface{} `bson:"body" json:"body"`
	Meta Meta                   `bson:"meta" json:"meta"`
}

// SpawnPayload is the body for REQ_SPAWN messages (Mitosis).
type SpawnPayload struct {
	RoleNeeded      string  `json:"role_needed"`
	TaskDescription string  `json:"task_description"`
	BudgetAllocated float64 `json:"budget_allocated"`
}

// DonePayload is the body for RPT_DONE messages (Delivery).
type DonePayload struct {
	ResultSummary   string     `json:"result_summary"`
	Artifacts       []Artifact `json:"artifacts"`
	ConfidenceScore float64    `json:"confidence_score"`
}

// Artifact represents a file produced by an agent.
type Artifact struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// MissionLog is an immutable record stored in the mission_logs collection.
type MissionLog struct {
	Timestamp   int64      `bson:"timestamp" json:"timestamp"`
	FromAgentID string     `bson:"from_agent_id" json:"from_agent_id"`
	ToAgentID   string     `bson:"to_agent_id" json:"to_agent_id"`
	PacketType  PacketType `bson:"packet_type" json:"packet_type"`
	Payload     string     `bson:"payload" json:"payload"`
	CostTokens  int        `bson:"cost_tokens" json:"cost_tokens"`
}
