package orchestrator

import (
	"log"
	"sync"
	"time"

	"essaim-backend/internal/core/economy"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
)

// WorkerCount defines the number of concurrent API workers (Architecture §7).
const WorkerCount = 50

// QueueSize defines the buffered channel capacity (Architecture §7).
const QueueSize = 1000

// ZombieTimeout defines the inactivity threshold for the Grim Reaper (§9.2).
const ZombieTimeout = 60 * time.Second

// ReaperInterval defines how often the Grim Reaper runs (§9.2).
const ReaperInterval = 5 * time.Second

// PacketHandler is a function that processes a received packet.
type PacketHandler func(pkt message.Packet) error

// Dispatcher is the central orchestrator managing the Worker Pool and message queue.
// It reads from a global channel and dispatches packets to agent goroutines.
type Dispatcher struct {
	mu             sync.RWMutex
	queue          chan message.Packet
	registry       *graph.Registry
	budgetMgr      *economy.BudgetManager
	handler        PacketHandler
	pendingBuffers map[string][]message.Packet // messages waiting for busy agents
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// NewDispatcher creates a new Dispatcher with the given dependencies.
func NewDispatcher(
	registry *graph.Registry,
	budgetMgr *economy.BudgetManager,
	handler PacketHandler,
) *Dispatcher {
	return &Dispatcher{
		queue:          make(chan message.Packet, QueueSize),
		registry:       registry,
		budgetMgr:      budgetMgr,
		handler:        handler,
		pendingBuffers: make(map[string][]message.Packet),
		stopCh:         make(chan struct{}),
	}
}

// Enqueue adds a packet to the global queue for processing.
func (d *Dispatcher) Enqueue(pkt message.Packet) {
	d.queue <- pkt
}

// Start launches the worker pool and the Grim Reaper.
func (d *Dispatcher) Start() {
	// Launch worker pool
	for i := 0; i < WorkerCount; i++ {
		d.wg.Add(1)
		go d.worker(i)
	}

	// Launch Grim Reaper (Garbage Collector)
	d.wg.Add(1)
	go d.grimReaper()

	log.Printf("[DISPATCHER] Started with %d workers, queue capacity %d", WorkerCount, QueueSize)
}

// Stop gracefully shuts down the dispatcher.
func (d *Dispatcher) Stop() {
	close(d.stopCh)
	d.wg.Wait()
	log.Println("[DISPATCHER] Stopped")
}

// worker processes packets from the queue.
func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()
	for {
		select {
		case <-d.stopCh:
			return
		case pkt := <-d.queue:
			d.processPacket(pkt)
		}
	}
}

func (d *Dispatcher) processPacket(pkt message.Packet) {
	// Panic recovery — never crash the server
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DISPATCHER] 🔥 PANIC recovered while processing packet %s for agent %s: %v", pkt.Head.ID, pkt.Head.To, r)
		}
	}()

	targetID := pkt.Head.To

	// Check if target agent exists
	node := d.registry.Get(targetID)
	if node == nil {
		log.Printf("[DISPATCHER] Target agent %s not found, dropping packet %s", targetID, pkt.Head.ID)
		return
	}

	// Check if target agent is working (busy) -> buffer the message
	if node.Agent.Status == agent.StatusWorking {
		d.mu.Lock()
		d.pendingBuffers[targetID] = append(d.pendingBuffers[targetID], pkt)
		d.mu.Unlock()
		log.Printf("[DISPATCHER] Agent %s is busy, buffering packet %s", targetID, pkt.Head.ID)
		return
	}

	// Process the packet via the handler
	if err := d.handler(pkt); err != nil {
		log.Printf("[DISPATCHER] Error processing packet %s for agent %s: %v", pkt.Head.ID, targetID, err)
	}
}

// grimReaper runs the Garbage Collector per Architecture §9.2.
func (d *Dispatcher) grimReaper() {
	defer d.wg.Done()
	ticker := time.NewTicker(ReaperInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.reap()
		}
	}
}

// reap checks all agents for zombie/bankrupt/depth-exceeded conditions.
func (d *Dispatcher) reap() {
	now := time.Now()
	nodes := d.registry.AllNodes()

	for _, node := range nodes {
		ag := node.Agent
		if ag.Status == agent.StatusDead {
			continue
		}

		shouldKill := false
		reason := ""

		// Rule 1: Zombie detection — DISABLED (laisse les agents travailler)
		// On ne tue plus les agents inactifs, ils peuvent attendre des réponses LLM lentes
		_ = now // keep import

		// Rule 2: Budget exhausted — DISABLED (on laisse les agents continuer)
		// On calibrera les budgets plus tard

		// Rule 3: Depth exceeded (max depth = 6)
		if node.Depth > graph.MaxDepth {
			shouldKill = true
			reason = "depth exceeded"
		}

		if shouldKill {
			log.Printf("[GRIM REAPER] Killing agent %s (%s): %s", ag.ID, ag.Role, reason)
			removed := d.registry.Remove(ag.ID)
			for _, id := range removed {
				d.budgetMgr.RemoveAgent(id)
			}
		}
	}
}

// DrainPending flushes the pending buffer for an agent that becomes idle.
func (d *Dispatcher) DrainPending(agentID string) {
	d.mu.Lock()
	pending, ok := d.pendingBuffers[agentID]
	if ok {
		delete(d.pendingBuffers, agentID)
	}
	d.mu.Unlock()

	for _, pkt := range pending {
		d.queue <- pkt
	}
}
