package agent

import "time"

// AgentStatus represents the state machine states of an Agent.
type AgentStatus string

const (
	StatusBorn    AgentStatus = "STATUS_BORN"
	StatusWorking AgentStatus = "STATUS_WORKING"
	StatusWaiting AgentStatus = "STATUS_WAITING"
	StatusReview  AgentStatus = "STATUS_REVIEW"
	StatusDead    AgentStatus = "STATUS_DEAD"
)

// AgentRole represents the specialization of an Agent.
type AgentRole string

const (
	RoleArchitect AgentRole = "ARCHITECT"
	RoleWorker    AgentRole = "WORKER"
	RoleCritic    AgentRole = "CRITIC"
	RoleCoder     AgentRole = "CODER"
)

// Message represents a single entry in the agent's memory (context window FIFO).
type Message struct {
	Role    string `bson:"role" json:"role"`       // "system", "user", "assistant"
	Content string `bson:"content" json:"content"` // JSON string payload
}

// Agent is the core entity of the ESSAIM system.
// Each active Goroutine holds one instance in memory.
// It is persisted to MongoDB (collection: agents_snapshot).
type Agent struct {
	ID          string      `bson:"_id" json:"id"`
	ParentID    string      `bson:"parent_id" json:"parentId"`
	Role        AgentRole   `bson:"role" json:"role"`
	Status      AgentStatus `bson:"status" json:"status"`
	Budget      float64     `bson:"budget" json:"budget"`
	Memory      []Message   `bson:"memory" json:"memory"`
	ChildrenIDs []string    `bson:"children_ids" json:"childrenIds"`
	CreatedAt   time.Time   `bson:"created_at" json:"createdAt"`
	UpdatedAt   time.Time   `bson:"updated_at" json:"updatedAt"`
}

// IsBankrupt returns true if the agent's budget is exhausted.
func (a *Agent) IsBankrupt() bool {
	return a.Budget <= 0
}

// IsAlive returns true if the agent is not dead.
func (a *Agent) IsAlive() bool {
	return a.Status != StatusDead
}

// AllocateBudget transfers a fraction of the agent's budget for a child.
// Returns the allocated amount and updates the parent's budget.
func (a *Agent) AllocateBudget(fraction float64) float64 {
	allocated := a.Budget * fraction
	a.Budget -= allocated
	return allocated
}
