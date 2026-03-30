package graph

import (
	"sync"

	"essaim-backend/internal/domain/agent"
)

// MaxDepth is the maximum allowed depth for the agent graph (Architecture Rule §9.2).
const MaxDepth = 6

// Node represents a single agent in the network.
type Node struct {
	Agent *agent.Agent
}

// Bubble represents a closed group of agents centered around a Postier.
// The group is "closed": the parent is NOT notified until ALL workers are done.
type Bubble struct {
	ID            string
	ParentAgentID string   // Who to notify when the bubble is complete
	PostierID     string   // The central Postier agent (may be empty for small groups)
	AgentIDs      []string // All member agent IDs (workers + postier)
	WorkerCount   int      // Total number of workers expected
	WorkersDone   int      // How many workers have reported done
	WorkResults   []string // Accumulated results from workers
}

// IsComplete returns true when all workers have reported done.
func (b *Bubble) IsComplete() bool {
	return b.WorkerCount > 0 && b.WorkersDone >= b.WorkerCount
}

// ResumeurCount returns how many Résumeurs this bubble needs based on its size.
// Rules from BROUILLON.md:
//   - <= 2  : no Postier, no Resumeur (direct parent reporting)
//   - 3-4   : 1 Postier, 0 Resumeur
//   - 5-8   : 1 Postier, 1 Resumeur
//   - 8-12  : 1 Postier, 2 Resumeurs
func ResumeurCount(workerCount int) int {
	switch {
	case workerCount <= 4:
		return 0
	case workerCount <= 8:
		return 1
	default:
		return 2
	}
}

// NeedsPostier returns true if a bubble of this size should have a Postier.
func NeedsPostier(workerCount int) bool {
	return workerCount >= 3
}

// Registry manages the in-memory graph of all active bubbles and agent nodes.
// It provides thread-safe operations for lookup, insertion, and deletion.
type Registry struct {
	mu      sync.RWMutex
	nodes   map[string]*Node   // key = Agent.ID
	bubbles map[string]*Bubble // key = BubbleID
}

// NewRegistry creates an empty node and bubble registry.
func NewRegistry() *Registry {
	return &Registry{
		nodes:   make(map[string]*Node),
		bubbles: make(map[string]*Bubble),
	}
}

// Register adds a new agent node to the graph and assigns it to its bubble.
func (r *Registry) Register(ag *agent.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add node
	r.nodes[ag.ID] = &Node{
		Agent: ag,
	}

	// Add to bubble if it has one
	if ag.BubbleID != "" {
		bubble, exists := r.bubbles[ag.BubbleID]
		if !exists {
			bubble = &Bubble{
				ID:       ag.BubbleID,
				AgentIDs: []string{},
			}
			r.bubbles[ag.BubbleID] = bubble
		}

		if ag.Role == agent.RolePostier {
			bubble.PostierID = ag.ID
		}

		// Avoid duplicate insertions (shouldn't happen but defensive)
		found := false
		for _, id := range bubble.AgentIDs {
			if id == ag.ID {
				found = true
				break
			}
		}
		if !found {
			bubble.AgentIDs = append(bubble.AgentIDs, ag.ID)
		}
	}

	return nil
}

// RegisterBubble explicitly creates a bubble with its parent and worker count.
// This must be called BEFORE spawning workers so the bubble context is established.
func (r *Registry) RegisterBubble(bubbleID, parentAgentID string, workerCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	bubble, exists := r.bubbles[bubbleID]
	if !exists {
		bubble = &Bubble{
			ID:       bubbleID,
			AgentIDs: []string{},
		}
		r.bubbles[bubbleID] = bubble
	}
	bubble.ParentAgentID = parentAgentID
	bubble.WorkerCount = workerCount
	bubble.WorkersDone = 0
	bubble.WorkResults = make([]string, 0, workerCount)
}

// RecordWorkerDone records a worker completing its task within a bubble.
// Returns (allDone bool, collected results) — allDone is true when the bubble is fully complete.
func (r *Registry) RecordWorkerDone(bubbleID, result string) (bool, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	bubble, exists := r.bubbles[bubbleID]
	if !exists {
		return false, nil
	}

	bubble.WorkersDone++
	bubble.WorkResults = append(bubble.WorkResults, result)

	if bubble.IsComplete() {
		// Return a copy of results
		results := make([]string, len(bubble.WorkResults))
		copy(results, bubble.WorkResults)
		return true, results
	}
	return false, nil
}

// Get returns the node for the given agent ID, or nil if not found.
func (r *Registry) Get(agentID string) *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes[agentID]
}

// GetBubble returns the bubble details for a given BubbleID.
func (r *Registry) GetBubble(bubbleID string) *Bubble {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bubbles[bubbleID]
}

// Remove deletes an agent from the node map and its bubble.
func (r *Registry) Remove(agentID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[agentID]
	if !ok {
		return nil
	}

	if node.Agent.BubbleID != "" {
		if bubble, exists := r.bubbles[node.Agent.BubbleID]; exists {
			// Remove agent ID from bubble list
			var newIDs []string
			for _, id := range bubble.AgentIDs {
				if id != agentID {
					newIDs = append(newIDs, id)
				}
			}
			bubble.AgentIDs = newIDs

			// Optional: delete bubble if empty
			if len(bubble.AgentIDs) == 0 {
				delete(r.bubbles, node.Agent.BubbleID)
			}
		}
	}

	delete(r.nodes, agentID)
	return []string{agentID}
}

// AllNodes returns a snapshot of all current nodes (for monitoring).
func (r *Registry) AllNodes() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		result = append(result, n)
	}
	return result
}

// Count returns the number of active nodes in the graph.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}
