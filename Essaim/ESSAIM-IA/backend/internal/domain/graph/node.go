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

// Bubble represents a group of agents centered around a Postier.
type Bubble struct {
	ID        string
	PostierID string
	AgentIDs  []string
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
// If the agent is a Postier, we might want to kill the whole bubble (returns the full list).
// For now, let's just remove the agent and return its ID. The logic for wiping a whole bubble
// belongs in the Lifecycle manager, knowing who is who.
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
