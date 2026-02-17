package graph

import (
	"fmt"
	"sync"

	"essaim-backend/internal/domain/agent"
)

// MaxDepth is the maximum allowed depth for the agent graph (Architecture Rule §9.2).
const MaxDepth = 6

// Node represents a single node in the directed agent graph.
type Node struct {
	Agent    *agent.Agent
	Children []*Node
	Depth    int
}

// Registry manages the in-memory graph of all active agent nodes.
// It provides thread-safe operations for node lookup, insertion, and deletion.
type Registry struct {
	mu    sync.RWMutex
	nodes map[string]*Node // key = Agent.ID
}

// NewRegistry creates an empty node registry.
func NewRegistry() *Registry {
	return &Registry{
		nodes: make(map[string]*Node),
	}
}

// Register adds a new agent node to the graph, linked to its parent.
// Returns an error if the max depth would be exceeded.
func (r *Registry) Register(ag *agent.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	depth := 0
	if ag.ParentID != "" {
		parent, ok := r.nodes[ag.ParentID]
		if !ok {
			return fmt.Errorf("parent node %s not found in registry", ag.ParentID)
		}
		depth = parent.Depth + 1
		if depth > MaxDepth {
			return fmt.Errorf("max depth %d exceeded for agent %s (depth=%d)", MaxDepth, ag.ID, depth)
		}
		parent.Children = append(parent.Children, &Node{Agent: ag, Depth: depth})
	}

	r.nodes[ag.ID] = &Node{
		Agent:    ag,
		Children: make([]*Node, 0),
		Depth:    depth,
	}
	return nil
}

// Get returns the node for the given agent ID, or nil if not found.
func (r *Registry) Get(agentID string) *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes[agentID]
}

// Remove deletes a node and recursively removes all its descendants.
// Returns a list of all removed agent IDs.
func (r *Registry) Remove(agentID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.removeRecursive(agentID)
}

func (r *Registry) removeRecursive(agentID string) []string {
	node, ok := r.nodes[agentID]
	if !ok {
		return nil
	}

	removed := []string{agentID}
	for _, child := range node.Children {
		removed = append(removed, r.removeRecursive(child.Agent.ID)...)
	}
	delete(r.nodes, agentID)
	return removed
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
