package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"essaim-backend/internal/core/economy"
	"essaim-backend/internal/core/evaluation"
	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/core/orchestrator"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
	"essaim-backend/internal/infrastructure/llm"
	"essaim-backend/internal/infrastructure/persistence"
	ws "essaim-backend/internal/infrastructure/websocket"
)

// Processor is the "brain" that connects incoming packets to LLM calls.
type Processor struct {
	ctx       context.Context
	registry  *graph.Registry
	budgetMgr *economy.BudgetManager
	llm       *llm.Client
	repo      *persistence.MongoRepo
	wsHub     *ws.Hub
	dispatch  *orchestrator.Dispatcher
	journal   *journal.Journal
	stats     *evaluation.MissionStats // auto-bilan metrics collector
}

// NewProcessor creates a new agent processor.
func NewProcessor(
	ctx context.Context,
	registry *graph.Registry,
	budgetMgr *economy.BudgetManager,
	llmClient *llm.Client,
	repo *persistence.MongoRepo,
	wsHub *ws.Hub,
	dispatch *orchestrator.Dispatcher,
	j *journal.Journal,
) *Processor {
	return &Processor{
		ctx:       ctx,
		registry:  registry,
		budgetMgr: budgetMgr,
		llm:       llmClient,
		repo:      repo,
		wsHub:     wsHub,
		dispatch:  dispatch,
		journal:   j,
		stats:     evaluation.NewMissionStats(""), // initialised with empty objective; set later via SetObjective
	}
}

// SetObjective updates the mission objective in the stats collector.
// Must be called before the first agent is spawned.
func (p *Processor) SetObjective(objective string) {
	p.stats = evaluation.NewMissionStats(objective)
}

// HandlePacket processes an incoming packet for an agent.
// This is the core logic of the system.
func (p *Processor) HandlePacket(pkt message.Packet) error {
	targetID := pkt.Head.To
	node := p.registry.Get(targetID)
	if node == nil {
		log.Printf("[PROCESSOR] ❌ Agent %s not found, dropping packet", targetID)
		return fmt.Errorf("agent %s not found", targetID)
	}

	ag := node.Agent
	log.Printf("[PROCESSOR] 📨 Processing %s for agent %s (%s)", pkt.Head.Type, ag.ID[:8], ag.Role)

	// Broadcast log to frontend
	p.broadcastLog(fmt.Sprintf("📨 Traitement %s pour %s (%s)", pkt.Head.Type, ag.ID[:8], ag.Role))

	switch pkt.Head.Type {
	case message.CmdTask:
		return p.handleTask(ag, pkt)
	case message.CmdKill:
		return p.killAgent(ag, "CMD_KILL reçu")
	case message.RptDone:
		return p.handleChildReport(ag, pkt)
	case message.RptFail:
		return p.handleChildReport(ag, pkt)
	default:
		log.Printf("[PROCESSOR] ⚠ Unhandled packet type: %s", pkt.Head.Type)
		return nil
	}
}

// handleTask processes a CMD_TASK by calling the LLM.
func (p *Processor) handleTask(ag *agent.Agent, pkt message.Packet) error {
	// Update agent status to WORKING
	ag.Status = agent.StatusWorking
	ag.UpdatedAt = time.Now()
	p.broadcastState(ag)

	// Check LLM client
	if p.llm == nil {
		log.Printf("[PROCESSOR] ❌ No LLM client available")
		p.broadcastLog(fmt.Sprintf("❌ Agent %s: pas de client LLM configuré", ag.ID[:8]))
		return fmt.Errorf("LLM client not available")
	}

	// Build the user message from the packet body
	objectiveRaw, _ := json.Marshal(pkt.Body)
	userMessage := string(objectiveRaw)

	// Journal: record task received
	if p.journal != nil {
		p.journal.Record(ag.ID, string(ag.Role), ag.BubbleID, journal.EntryTaskReceived, userMessage)
	}

	// Generate the system prompt
	systemPrompt := agent.GenerateSystemPrompt(ag)

	// Add task context to memory
	ag.Memory = append(ag.Memory, agent.Message{
		Role:    "user",
		Content: userMessage,
	})

	log.Printf("[PROCESSOR] 🤖 Calling LLM for agent %s (%s)...", ag.ID[:8], ag.Role)
	p.broadcastLog(fmt.Sprintf("🤖 Appel LLM pour %s (%s)...", ag.ID[:8], ag.Role))

	var resp *llm.LLMResponse
	var err error

	// Call the LLM normally
	resp, err = p.llm.Call(p.ctx, systemPrompt, userMessage)

	if err != nil {
		log.Printf("[PROCESSOR] ❌ LLM call failed for agent %s: %v", ag.ID[:8], err)
		p.broadcastLog(fmt.Sprintf("❌ Erreur LLM pour %s: %v", ag.ID[:8], err))

		// Send failure report to parent
		p.reportToParent(ag, message.RptFail, map[string]interface{}{
			"error": err.Error(),
		})

		// Mark agent as dead so parent doesn't wait forever
		ag.Status = agent.StatusDead
		ag.UpdatedAt = time.Now()
		p.broadcastState(ag)

		return err
	}

	log.Printf("[PROCESSOR] ✅ LLM response for %s: %d tokens, %dms", ag.ID[:8], resp.TokensUsed, resp.LatencyMs)
	log.Printf("[PROCESSOR] 📄 Raw LLM output for %s:\n%s", ag.ID[:8], resp.RawJSON)
	p.broadcastLog(fmt.Sprintf("✅ Réponse LLM pour %s: %d tokens, %dms", ag.ID[:8], resp.TokensUsed, resp.LatencyMs))

	// Journal: record LLM response
	if p.journal != nil {
		p.journal.Record(ag.ID, string(ag.Role), ag.BubbleID, journal.EntryLLMResponse, resp.RawJSON)
	}

	// Deduct tokens from budget
	bankrupt, err := p.budgetMgr.Deduct(ag.ID, float64(resp.TokensUsed))
	if err != nil {
		log.Printf("[PROCESSOR] ⚠ Budget deduction error: %v", err)
	}
	ag.Budget, _ = p.budgetMgr.GetBudget(ag.ID)

	if bankrupt {
		log.Printf("[PROCESSOR] ⚠ Agent %s budget épuisé — continue quand même", ag.ID[:8])
		p.broadcastLog(fmt.Sprintf("⚠ Agent %s budget bas — continue", ag.ID[:8]))
	}

	// Save LLM response to memory
	ag.Memory = append(ag.Memory, agent.Message{
		Role:    "assistant",
		Content: resp.RawJSON,
	})

	// Parse the LLM response
	var action LLMAction
	if err := json.Unmarshal([]byte(resp.RawJSON), &action); err != nil {
		log.Printf("[PROCESSOR] ❌ Failed to parse LLM response: %v", err)
		log.Printf("[PROCESSOR] Raw response: %s", resp.RawJSON)
		p.broadcastLog(fmt.Sprintf("❌ Réponse LLM invalide pour %s: %v", ag.ID[:8], err))

		p.reportToParent(ag, message.RptFail, map[string]interface{}{
			"error": "invalid LLM response format",
			"raw":   resp.RawJSON,
		})
		return err
	}

	// Log mission
	if p.repo != nil {
		logEntry := &message.MissionLog{
			Timestamp:   time.Now().Unix(),
			FromAgentID: ag.ID,
			ToAgentID:   ag.BubbleID,
			PacketType:  pkt.Head.Type,
			Payload:     resp.RawJSON,
			CostTokens:  int(resp.TokensUsed),
		}
		p.repo.InsertMissionLog(p.ctx, logEntry)
	}

	// Normalize the action name (LLMs are creative with naming)
	normalizedAction := normalizeAction(action.Action)
	log.Printf("[PROCESSOR] 🎯 Action: %s (normalized from: %s)", normalizedAction, action.Action)

	// Process the action
	switch normalizedAction {
	case "SPAWN":
		return p.handleSpawn(ag, action.Payload)
	case "WORK":
		return p.handleWork(ag, action.Payload)
	case "REPORT":
		return p.handleReport(ag, action.Payload)
	default:
		log.Printf("[PROCESSOR] ⚠ Unknown action: %s (normalized: %s)", action.Action, normalizedAction)
		p.broadcastLog(fmt.Sprintf("⚠ Action inconnue de %s: %s", ag.ID[:8], action.Action))
		// Treat as a WORK with the raw payload
		return p.handleWork(ag, action.Payload)
	}
}
