package routing

import (
	"log"

	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
)

// Router manages message passing within a Bubble.
type Router struct {
	Registry *graph.Registry
}

// NewRouter creates a new Bubble router.
func NewRouter(registry *graph.Registry) *Router {
	return &Router{
		Registry: registry,
	}
}

// RoutePacket decides where a packet should go within a bubble.
// If the target is strictly defined, it goes there.
// If it's a broadcast or needs Postier interception, routing modifies the target.
func (r *Router) RoutePacket(pkt *message.Packet) {
	// 1. Identify source constraints
	sourceNode := r.Registry.Get(pkt.Head.From)
	if sourceNode == nil {
		return // Source doesn't exist
	}

	sourceBubble := r.Registry.GetBubble(sourceNode.Agent.BubbleID)

	// 2. Default rule: Workers must talk to the Postier.
	// If a Worker is trying to talk directly to another Worker or Architect, intercept it.
	if sourceNode.Agent.Role == agent.RoleWorker {
		if sourceBubble != nil && sourceBubble.PostierID != "" {
			if pkt.Head.To != sourceBubble.PostierID {
				log.Printf("[ROUTING] Intercepting message from Worker %s to %s, redirecting to Postier %s", pkt.Head.From, pkt.Head.To, sourceBubble.PostierID)
				pkt.Head.To = sourceBubble.PostierID
			}
		}
	}

	// Postiers and Architects have free routing within constraints (managed elsewhere).
}
