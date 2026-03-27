package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"essaim-backend/internal/core/economy"
	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/core/orchestrator"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
	"essaim-backend/internal/infrastructure/llm"
	"essaim-backend/internal/infrastructure/persistence"
	ws "essaim-backend/internal/infrastructure/websocket"

	"github.com/google/uuid"
)

// Processor is the "brain" that connects incoming packets to LLM calls.
// When a CMD_TASK arrives for an agent, the processor:
// 1. Builds the system prompt for the agent
// 2. Calls the LLM
// 3. Parses the JSON response (SPAWN / WORK / REPORT)
// 4. Takes action: create children, report results, etc.
type Processor struct {
	ctx       context.Context
	registry  *graph.Registry
	budgetMgr *economy.BudgetManager
	llm       *llm.Client
	repo      *persistence.MongoRepo
	wsHub     *ws.Hub
	dispatch  *orchestrator.Dispatcher
	journal   *journal.Journal
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
	}
}

// LLMAction is the expected top-level action from the LLM response.
type LLMAction struct {
	Action  string          `json:"action"`  // "SPAWN", "WORK", "REPORT"
	Payload json.RawMessage `json:"payload"` // variable structure
}

// SpawnPayload for action=SPAWN
type SpawnPayload struct {
	Subtasks []SubtaskDef `json:"subtasks"`
}

// SubtaskDef defines a sub-agent to spawn.
type SubtaskDef struct {
	Role            string  `json:"role"`
	TaskDescription string  `json:"task_description"`
	BudgetFraction  float64 `json:"budget_fraction"`
}

// WorkPayload for action=WORK
type WorkPayload struct {
	Result     string  `json:"result"`
	Confidence float64 `json:"confidence"`
}

// ReportPayload for action=REPORT
type ReportPayload struct {
	Summary    string           `json:"result_summary"`
	Artifacts  []ArtifactOutput `json:"artifacts"`
	Confidence float64          `json:"confidence_score"`
}

// ArtifactOutput is a file produced by the agent.
type ArtifactOutput struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
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

// handleChildReport processes RPT_DONE / RPT_FAIL from a child agent.
// It routes differently depending on whether the destination is a Postier or a direct parent.
func (p *Processor) handleChildReport(receiver *agent.Agent, pkt message.Packet) error {
	childID := pkt.Head.From
	log.Printf("[PROCESSOR] 📩 %s (%s) received %s from child %s", receiver.ID[:8], receiver.Role, pkt.Head.Type, childID[:8])

	// PATH A: The receiver is a Postier — use the closed-group (Bulle) aggregation path.
	if receiver.Role == agent.RolePostier {
		return p.handlePostierReport(receiver, pkt)
	}

	// PATH B: Small group (<=2 workers) — no Postier, direct parent aggregation (legacy path).
	return p.handleDirectParentReport(receiver, pkt)
}

// handlePostierReport is called when a Worker reports to its Postier.
// The Postier collects results and triggers the Résumeur when the bubble is complete.
func (p *Processor) handlePostierReport(postier *agent.Agent, pkt message.Packet) error {
	childID := pkt.Head.From

	// Extract the worker result text
	resultJSON, _ := json.MarshalIndent(pkt.Body, "", "  ")
	childResultText := extractResultText(resultJSON)

	log.Printf("[POSTIER] 📬 Postier %s received result from Worker %s (bubble: %s)", postier.ID[:8], childID[:8], postier.BubbleID)
	p.broadcastLog(fmt.Sprintf("📬 Postier %s : résultat reçu de Worker %s", postier.ID[:8], childID[:8]))

	// Record the done worker in the bubble registry
	allDone, results := p.registry.RecordWorkerDone(postier.BubbleID, childResultText)

	if !allDone {
		bubble := p.registry.GetBubble(postier.BubbleID)
		var remaining int
		if bubble != nil {
			remaining = bubble.WorkerCount - bubble.WorkersDone
		}
		p.broadcastLog(fmt.Sprintf("⏳ Postier %s attend %d workers restants...", postier.ID[:8], remaining))
		return nil
	}

	// ALL workers done — trigger synthesis
	log.Printf("[POSTIER] ✅ Bubble %s complete (%d workers). Triggering synthesis.", postier.BubbleID, len(results))
	p.broadcastLog(fmt.Sprintf("✅ Bulle fermée ! %d workers terminés — démarrage de la synthèse...", len(results)))

	// Get the parent agent ID from the bubble
	bubble := p.registry.GetBubble(postier.BubbleID)
	if bubble == nil {
		log.Printf("[POSTIER] ❌ Bubble %s not found after completion — cannot escalate", postier.BubbleID)
		return fmt.Errorf("bubble %s not found", postier.BubbleID)
	}
	parentAgentID := bubble.ParentAgentID

	// Journal: record synthesis trigger
	if p.journal != nil {
		p.journal.Record(postier.ID, string(agent.RolePostier), postier.BubbleID, journal.EntrySynthesis,
			fmt.Sprintf("Bulle fermée. %d workers terminés. Démarrage synthèse vers parent %s.", len(results), parentAgentID[:8]))
	}

	// Run Résumeur(s) according to group-size rules
	numWorkers := len(results)
	var finalSummary string

	switch {
	case numWorkers <= 4:
		// 3-4 workers: Postier only, no Résumeur — direct concatenation
		log.Printf("[POSTIER] 🔄 %d workers (<=4) — no Résumeur, direct concatenation", numWorkers)
		p.broadcastLog(fmt.Sprintf("🔄 %d workers — concaténation directe (pas de Résumeur)", numWorkers))
		for idx, res := range results {
			finalSummary += fmt.Sprintf("\n\n--- WORKER %d ---\n\n%s", idx+1, res)
		}

	case numWorkers <= 8:
		// 5-8 workers: 1 Résumeur
		log.Printf("[POSTIER] 🔄 %d workers (5-8) — invoking 1 Résumeur", numWorkers)
		p.broadcastLog(fmt.Sprintf("🔄 %d workers — appel de 1 Résumeur...", numWorkers))
		var allText string
		for idx, res := range results {
			allText += fmt.Sprintf("\n\n--- WORKER %d ---\n\n%s", idx+1, res)
		}
		finalSummary = p.callResumeur(postier, allText)

	default:
		// 9-12 workers: 2 Résumeurs (applied to each half)
		log.Printf("[POSTIER] 🔄 %d workers (>8) — invoking 2 Résumeurs", numWorkers)
		p.broadcastLog(fmt.Sprintf("🔄 %d workers — appel de 2 Résumeurs séquentiels...", numWorkers))
		half := numWorkers / 2
		var part1, part2 string
		for idx, res := range results[:half] {
			part1 += fmt.Sprintf("\n\n--- WORKER %d ---\n\n%s", idx+1, res)
		}
		for idx, res := range results[half:] {
			part2 += fmt.Sprintf("\n\n--- WORKER %d ---\n\n%s", half+idx+1, res)
		}
		sum1 := p.callResumeur(postier, part1)
		sum2 := p.callResumeur(postier, part2)
		finalSummary = fmt.Sprintf("=== SYNTHÈSE PARTIE 1 ===\n%s\n\n=== SYNTHÈSE PARTIE 2 ===\n%s", sum1, sum2)
	}

	// Find the parent node and escalate
	parentNode := p.registry.Get(parentAgentID)
	if parentNode == nil {
		log.Printf("[POSTIER] ❌ Parent agent %s not found — escalating to frontend only", parentAgentID[:8])
		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventSystemAlert,
			Payload: map[string]interface{}{
				"message": fmt.Sprintf("Bulle %s terminée (parent introuvable)", postier.BubbleID),
				"result":  finalSummary,
			},
		})
		return nil
	}

	// Send RPT_DONE to the parent via the dispatcher
	p.broadcastLog(fmt.Sprintf("⬆️ Postier %s remonte la synthèse au parent %s (%s)", postier.ID[:8], parentAgentID[:8], parentNode.Agent.Role))
	reportPkt := message.Packet{
		Head: message.Header{
			ID:        uuid.New().String(),
			Timestamp: time.Now().Unix(),
			From:      postier.ID,
			To:        parentAgentID,
			Type:      message.RptDone,
		},
		Body: map[string]interface{}{
			"result_summary": finalSummary,
			"from_bubble":    postier.BubbleID,
			"worker_count":   numWorkers,
		},
	}
	p.dispatch.Enqueue(reportPkt)

	// Mark Postier as done
	postier.Status = agent.StatusDead
	postier.UpdatedAt = time.Now()
	p.broadcastState(postier)
	return nil
}

// handleDirectParentReport handles child reports for small groups (<=2 workers) without a Postier.
// This is the legacy path for tiny delegations that don't need a routing hub.
func (p *Processor) handleDirectParentReport(parent *agent.Agent, pkt message.Packet) error {
	childID := pkt.Head.From

	// Store child result in parent memory
	resultJSON, _ := json.MarshalIndent(pkt.Body, "", "  ")
	parent.Memory = append(parent.Memory, agent.Message{
		Role:    "user",
		Content: fmt.Sprintf("[CHILD_REPORT from %s (%s)]:\n%s", childID[:8], pkt.Head.Type, string(resultJSON)),
	})
	parent.UpdatedAt = time.Now()
	parent.SubtasksPending--

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, parent)
	}

	if parent.SubtasksPending > 0 {
		p.broadcastLog(fmt.Sprintf("⏳ %s attend d'autres sous-agents (%d restants)...", parent.ID[:8], parent.SubtasksPending))
		return nil
	}
	if parent.SubtasksPending < 0 {
		return nil
	}

	// All done — extract results and concatenate (no Résumeur for groups <= 2)
	var childResults []string
	for _, msg := range parent.Memory {
		if strings.HasPrefix(msg.Content, "[CHILD_REPORT") {
			parts := strings.SplitN(msg.Content, "]:\n", 2)
			if len(parts) == 2 {
				childResults = append(childResults, extractResultText([]byte(parts[1])))
			}
		}
	}

	var finalSummary string
	for idx, res := range childResults {
		finalSummary += fmt.Sprintf("\n\n--- OUTPUT DE L'AGENT %d ---\n\n%s", idx+1, res)
	}

	p.reportToParent(parent, message.RptDone, map[string]interface{}{
		"result_summary": finalSummary,
	})

	parent.Status = agent.StatusDead
	parent.UpdatedAt = time.Now()
	p.broadcastState(parent)
	return nil
}

// extractResultText extracts the human-readable result from a JSON result payload.
func extractResultText(resultJSON []byte) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal(resultJSON, &parsed); err == nil {
		if res, ok := parsed["result"].(string); ok && res != "" {
			return res
		}
		if sum, ok := parsed["result_summary"].(string); ok && sum != "" {
			return sum
		}
	}
	return string(resultJSON)
}

func (p *Processor) callResumeur(parent *agent.Agent, content string) string {
	resumeurPrompt := fmt.Sprintf(
		"Voici les productions terminées d'une partie du sous-groupe.\n"+
			"TA TÂCHE : Lis l'intégralité de ces travaux, crée un compte-rendu consolidé et intelligent (Executive Summary) "+
			"pour le Manager de ce groupe. Garde l'essence, les décisions clés, les pépites et le code si présent, mais retire le bruit inutile.\n\n%s\n\n"+
			"Réponds en JSON strict : {\"action\": \"REPORT\", \"payload\": {\"result_summary\": \"[TON RÉSUMÉ CONSOLIDÉ ICI]\", \"artifacts\": [], \"confidence_score\": 1.0}}",
		content,
	)

	virtualResumeur := &agent.Agent{
		ID:       parent.ID + "-RESUMEUR-" + fmt.Sprintf("%d", time.Now().UnixNano()%10000),
		Role:     agent.RoleResumeur,
		BubbleID: parent.BubbleID,
		Budget:   parent.Budget,
	}

	systemPrompt := agent.GenerateSystemPrompt(virtualResumeur)

	resp, err := p.llm.Call(p.ctx, systemPrompt, resumeurPrompt)
	if err != nil {
		log.Printf("[PROCESSOR] ❌ Resumeur LLM call failed for %s: %v", parent.ID[:8], err)
		return "Erreur lors de la Synthèse: Echec du LLM."
	}

	if p.journal != nil {
		p.journal.Record(parent.ID, string(agent.RoleResumeur), parent.BubbleID, journal.EntrySynthesis, resp.RawJSON)
	}

	p.budgetMgr.Deduct(parent.ID, float64(resp.TokensUsed))

	var action LLMAction
	if err := json.Unmarshal([]byte(resp.RawJSON), &action); err != nil {
		return resp.RawJSON
	}

	var payloadMap map[string]interface{}
	if err := json.Unmarshal(action.Payload, &payloadMap); err == nil {
		if res, ok := payloadMap["result_summary"].(string); ok {
			return res
		}
	}

	return string(action.Payload)
}

// handleSpawn creates child agents from the LLM's SPAWN action.
// BUBBLE SYSTEM: If N >= 3 workers, auto-creates a Postier agent and registers a closed Bubble.
// Workers are then told to report to the Postier, not the parent.
func (p *Processor) handleSpawn(parent *agent.Agent, payload json.RawMessage) error {
	log.Printf("[PROCESSOR] 🔍 SPAWN payload raw: %s", string(payload))

	subtasks := p.parseSubtasks(payload)

	// Enforce maximum of 12 sub-agents spawned by a single agent
	if len(subtasks) > 12 {
		log.Printf("[PROCESSOR] ⚠ Agent %s attempted to spawn %d children, capping at 12.", parent.ID[:8], len(subtasks))
		p.broadcastLog(fmt.Sprintf("⚠ %s tente de créer %d enfants (limite 12) — Troncature.", parent.ID[:8], len(subtasks)))
		subtasks = subtasks[:12]
	}

	numWorkers := len(subtasks)
	log.Printf("[PROCESSOR] 🔀 Agent %s spawning %d children", parent.ID[:8], numWorkers)
	p.broadcastLog(fmt.Sprintf("🔀 %s (%s) crée %d sous-agents", parent.ID[:8], parent.Role, numWorkers))

	if p.journal != nil {
		var spawnInfo string
		for _, t := range subtasks {
			spawnInfo += fmt.Sprintf("- **%s** : %s\n", t.Role, t.TaskDescription)
		}
		p.journal.Record(parent.ID, string(parent.Role), parent.BubbleID, journal.EntrySpawn,
			fmt.Sprintf("Création de %d sous-agents :\n%s", numWorkers, spawnInfo))
	}

	parent.Status = agent.StatusWaiting
	parent.UpdatedAt = time.Now()
	p.broadcastState(parent)

	// --- BUBBLE SYSTEM ---
	// Determine if we need a Postier hub for this group.
	needsPostier := graph.NeedsPostier(numWorkers)

	// Generate a fresh, unique bubbleID for this closed group.
	bubbleID := uuid.New().String()

	// Register the bubble in the Registry BEFORE spawning workers.
	p.registry.RegisterBubble(bubbleID, parent.ID, numWorkers)

	// Create the Postier agent if the group is large enough.
	var postierID string
	if needsPostier {
		postierBudget, err := p.budgetMgr.AllocateFromParent(parent.ID, 0.05) // Postier gets 5% budget
		if err != nil {
			log.Printf("[PROCESSOR] ⚠ Budget allocation for Postier failed: %v — proceeding without Postier", err)
			needsPostier = false
		} else {
			postier := &agent.Agent{
				ID:        uuid.New().String(),
				BubbleID:  bubbleID,
				Role:      agent.RolePostier,
				Status:    agent.StatusWaiting,
				Budget:    postierBudget,
				Memory:    make([]agent.Message, 0),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			postierID = postier.ID

			// Update the bubble's PostierID
			if b := p.registry.GetBubble(bubbleID); b != nil {
				b.PostierID = postierID
			}

			if err := p.registry.Register(postier); err != nil {
				log.Printf("[PROCESSOR] ❌ Failed to register Postier: %v", err)
				needsPostier = false
			} else {
				p.budgetMgr.RegisterAgent(postierID, postierBudget)
				if p.repo != nil {
					p.repo.UpsertAgent(p.ctx, postier)
				}
				p.wsHub.Broadcast(ws.Event{
					Type: ws.EventGraphUpdate,
					Payload: map[string]interface{}{
						"action": "ADD_NODE",
						"agent":  postier,
					},
				})
				log.Printf("[POSTIER] 🏤 Postier %s created for bubble %s (parent: %s)", postierID[:8], bubbleID[:8], parent.ID[:8])
				p.broadcastLog(fmt.Sprintf("🏤 Postier créé pour la bulle (%d workers)", numWorkers))
			}
		}
	}

	// Parent waits for ONE report (from the Postier, or directly from children if no Postier)
	if needsPostier {
		// Parent has exactly 1 pending subtask: the Postier will deliver one consolidated result.
		parent.SubtasksPending = 1
	} else {
		// No Postier — parent waits for all individual workers directly.
		parent.SubtasksPending = numWorkers
	}

	// Calculate fair budget fraction for workers
	fairFraction := 0.8 / float64(numWorkers)
	if fairFraction > 0.5 {
		fairFraction = 0.5
	}
	if fairFraction < 0.001 {
		fairFraction = 0.001
	}

	// Spawn all worker agents
	spawnedCount := 0
	for i, task := range subtasks {
		fraction := task.BudgetFraction
		if fraction <= 0 || fraction > 0.8 {
			fraction = fairFraction
		}

		childBudget, err := p.budgetMgr.AllocateFromParent(parent.ID, fraction)
		if err != nil {
			log.Printf("[PROCESSOR] ⚠ Budget allocation failed for child %d: %v", i, err)
			p.broadcastLog(fmt.Sprintf("⚠ Budget insuffisant pour créer le sous-agent %d", i+1))
			// If parent is waiting for this worker (no Postier), decrement pending
			if !needsPostier {
				parent.SubtasksPending--
			}
			// In Postier mode, update bubble worker count
			if needsPostier {
				if b := p.registry.GetBubble(bubbleID); b != nil {
					b.WorkerCount--
				}
			}
			continue
		}

		role := agent.AgentRole(task.Role)
		if role == "" {
			role = agent.RoleWorker
		}

		child := &agent.Agent{
			ID:        uuid.New().String(),
			BubbleID:  bubbleID,
			PostierID: postierID, // Empty string if no Postier (<=2 workers)
			Role:      role,
			Status:    agent.StatusBorn,
			Budget:    childBudget,
			Memory:    make([]agent.Message, 0),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := p.registry.Register(child); err != nil {
			log.Printf("[PROCESSOR] ❌ Failed to register child: %v", err)
			p.broadcastLog(fmt.Sprintf("❌ Échec enregistrement sous-agent: %v", err))
			if !needsPostier {
				parent.SubtasksPending--
			}
			continue
		}

		if err := p.budgetMgr.RegisterAgent(child.ID, childBudget); err != nil {
			log.Printf("[PROCESSOR] ❌ Failed to register child budget: %v", err)
			continue
		}

		if p.repo != nil {
			p.repo.UpsertAgent(p.ctx, child)
		}

		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventGraphUpdate,
			Payload: map[string]interface{}{
				"action": "ADD_NODE",
				"agent":  child,
			},
		})

		taskDesc := task.TaskDescription
		log.Printf("[PROCESSOR] ✅ Spawned child %s (%s) → reports to %s",
			child.ID[:8], child.Role,
			func() string {
				if postierID != "" {
					return "Postier " + postierID[:8]
				}
				return "Parent " + parent.ID[:8]
			}())
		p.broadcastLog(fmt.Sprintf("✅ Worker %s (%s) créé: %s", child.ID[:8], child.Role, truncate(taskDesc, 60)))

		// Determine where the worker should send its RPT_DONE:
		// → to the Postier if one exists, otherwise directly to the parent.
		reportTarget := parent.ID
		if needsPostier && postierID != "" {
			reportTarget = postierID
		}

		taskPkt := message.Packet{
			Head: message.Header{
				ID:        uuid.New().String(),
				Timestamp: time.Now().Unix(),
				From:      parent.ID,
				To:        child.ID,
				Type:      message.CmdTask,
			},
			Body: map[string]interface{}{
				"task_description": taskDesc,
				"from_parent":      parent.ID,
				"parent_role":      string(parent.Role),
				"report_to":        reportTarget, // Tells the worker where to report
			},
		}
		p.dispatch.Enqueue(taskPkt)
		spawnedCount++
	}

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, parent)
	}

	// FALLBACK: if no children were successfully spawned, do the work directly.
	if spawnedCount == 0 {
		log.Printf("[PROCESSOR] ⚠ Agent %s SPAWN failed (0 children created) — falling back to WORK", parent.ID[:8])
		p.broadcastLog(fmt.Sprintf("⚠ %s n'a pu créer aucun sous-agent — fait le travail lui-même", parent.ID[:8]))

		parent.Status = agent.StatusWorking
		parent.SubtasksPending = 0
		parent.UpdatedAt = time.Now()
		p.broadcastState(parent)

		var combinedTask string
		for _, task := range subtasks {
			combinedTask += "- " + task.TaskDescription + "\n"
		}

		workPrompt := fmt.Sprintf("Tu n'as pas assez de budget pour déléguer. FAIS LE TRAVAIL TOI-MÊME.\n"+
			"Voici les tâches à accomplir :\n%s\n"+
			"Réponds avec {\"action\": \"WORK\", \"payload\": {\"result\": \"...\", \"confidence\": 0.0-1.0}}", combinedTask)

		systemPrompt := agent.GenerateSystemPrompt(parent)
		resp, err := p.llm.Call(p.ctx, systemPrompt, workPrompt)
		if err != nil {
			log.Printf("[PROCESSOR] ❌ Fallback WORK LLM call failed: %v", err)
			p.reportToParent(parent, message.RptFail, map[string]interface{}{
				"error": "spawn failed and fallback work failed: " + err.Error(),
			})
			parent.Status = agent.StatusDead
			parent.UpdatedAt = time.Now()
			p.broadcastState(parent)
			return err
		}

		p.budgetMgr.Deduct(parent.ID, float64(resp.TokensUsed))
		parent.Budget, _ = p.budgetMgr.GetBudget(parent.ID)

		var action LLMAction
		if err := json.Unmarshal([]byte(resp.RawJSON), &action); err == nil {
			return p.handleWork(parent, action.Payload)
		}
		return p.handleWork(parent, json.RawMessage(fmt.Sprintf(`{"result": %s, "confidence": 0.7}`, resp.RawJSON)))
	}

	return nil
}

// parseSubtasks tries multiple strategies to extract subtask definitions from LLM payload.
func (p *Processor) parseSubtasks(payload json.RawMessage) []SubtaskDef {
	// Strategy 1: direct {"subtasks": [...]}
	var sp SpawnPayload
	if err := json.Unmarshal(payload, &sp); err == nil && len(sp.Subtasks) > 0 {
		log.Printf("[PROCESSOR] ✅ Parsed %d subtasks (direct)", len(sp.Subtasks))
		return sp.Subtasks
	}

	// Strategy 2: payload is a flat array of subtasks [...]
	var arr []SubtaskDef
	if err := json.Unmarshal(payload, &arr); err == nil && len(arr) > 0 {
		log.Printf("[PROCESSOR] ✅ Parsed %d subtasks (array)", len(arr))
		return arr
	}

	// Strategy 3: payload is a single subtask object {"role": ..., "task_description": ...}
	var single SubtaskDef
	if err := json.Unmarshal(payload, &single); err == nil && single.TaskDescription != "" {
		log.Printf("[PROCESSOR] ✅ Parsed 1 subtask (single)")
		return []SubtaskDef{single}
	}

	// Strategy 4: generic map — look for known keys
	var rawMap map[string]interface{}
	if err := json.Unmarshal(payload, &rawMap); err == nil {
		// Check nested arrays with various key names
		for _, key := range []string{"subtasks", "tasks", "sub_tasks", "children", "agents", "bubbles"} {
			if val, ok := rawMap[key]; ok {
				raw, _ := json.Marshal(val)
				var nested []SubtaskDef
				if err := json.Unmarshal(raw, &nested); err == nil && len(nested) > 0 {
					// Validate that at least one subtask has a non-empty TaskDescription
					hasValid := false
					for _, s := range nested {
						if s.TaskDescription != "" {
							hasValid = true
							break
						}
					}
					if hasValid {
						log.Printf("[PROCESSOR] ✅ Parsed %d subtasks (nested key: %s)", len(nested), key)
						return nested
					}
					log.Printf("[PROCESSOR] ⚠ Nested key %s found %d items but none had task_description, trying generic map parser", key, len(nested))
				}
				// Try as array of generic maps
				var items []map[string]interface{}
				if err := json.Unmarshal(raw, &items); err == nil {
					var results []SubtaskDef
					for _, item := range items {
						route := p.extractSubtaskFromMap(item)
						if route.TaskDescription != "" {
							results = append(results, route)
						}
					}
					if len(results) > 0 {
						log.Printf("[PROCESSOR] ✅ Parsed %d subtasks (generic maps in key: %s)", len(results), key)
						return results
					}
				}
			}
		}
		// Fallback: try to extract a single task from the map itself
		single := p.extractSubtaskFromMap(rawMap)
		if single.TaskDescription != "" {
			log.Printf("[PROCESSOR] ✅ Parsed 1 subtask (map fallback)")
			return []SubtaskDef{single}
		}
	}

	log.Printf("[PROCESSOR] ❌ Could not parse any subtasks from payload")
	return nil
}

// extractSubtaskFromMap tries to extract task info from a generic map.
func (p *Processor) extractSubtaskFromMap(m map[string]interface{}) SubtaskDef {
	desc := ""
	for _, key := range []string{"task_description", "description", "task", "instruction", "instructions", "objective"} {
		if v, ok := m[key].(string); ok && v != "" {
			desc = v
			break
		}
	}
	role := "WORKER"
	for _, key := range []string{"role", "role_needed", "agent_role", "type", "responsible"} {
		if v, ok := m[key].(string); ok && v != "" {
			role = v
			break
		}
	}
	fraction := 0.2
	for _, key := range []string{"budget_fraction", "budget_allocated", "budget"} {
		if v, ok := m[key].(float64); ok && v > 0 {
			fraction = v
			if fraction > 1 {
				fraction = 0.2
			}
			break
		}
	}
	return SubtaskDef{Role: role, TaskDescription: desc, BudgetFraction: fraction}
}

// handleWork processes a direct WORK response from the LLM.
func (p *Processor) handleWork(ag *agent.Agent, payload json.RawMessage) error {
	var work WorkPayload
	json.Unmarshal(payload, &work)

	// If "result" is empty, extract content from any key in the payload
	if work.Result == "" {
		var rawMap map[string]interface{}
		if err := json.Unmarshal(payload, &rawMap); err == nil {
			// Pretty-print the entire payload as the result
			pretty, _ := json.MarshalIndent(rawMap, "", "  ")
			work.Result = string(pretty)
		}
	}

	log.Printf("[PROCESSOR] 📝 Agent %s completed work (confidence: %.0f%%)", ag.ID[:8], work.Confidence*100)
	log.Printf("[PROCESSOR] 📄 WORK RESULT from %s:\n%s", ag.ID[:8], work.Result)
	p.broadcastLog(fmt.Sprintf("📝 %s (%s) a terminé son travail (confiance: %.0f%%)", ag.ID[:8], ag.Role, work.Confidence*100))

	// Journal: record work content
	if p.journal != nil {
		p.journal.Record(ag.ID, string(ag.Role), ag.BubbleID, journal.EntryWork, fmt.Sprintf("**Confiance : %.0f%%**\n\n%s", work.Confidence*100, work.Result))
	}

	// Report to parent
	p.reportToParent(ag, message.RptDone, map[string]interface{}{
		"result":     work.Result,
		"confidence": work.Confidence,
		"agent_id":   ag.ID,
		"role":       ag.Role,
	})

	// Agent is done with this task — mark as done but DON'T destroy
	ag.Status = agent.StatusDead
	ag.UpdatedAt = time.Now()
	p.broadcastState(ag)

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, ag)
	}

	return nil
}

// handleReport processes a REPORT response from the LLM.
func (p *Processor) handleReport(ag *agent.Agent, payload json.RawMessage) error {
	var report ReportPayload
	json.Unmarshal(payload, &report)

	// If result_summary is empty, extract from any key in the payload
	if report.Summary == "" {
		var rawMap map[string]interface{}
		if err := json.Unmarshal(payload, &rawMap); err == nil {
			pretty, _ := json.MarshalIndent(rawMap, "", "  ")
			report.Summary = string(pretty)
		}
	}

	log.Printf("[PROCESSOR] 📋 Agent %s reporting (confidence: %.0f%%)", ag.ID[:8], report.Confidence*100)
	log.Printf("[PROCESSOR] 📄 REPORT from %s:\n%s", ag.ID[:8], report.Summary)
	if len(report.Artifacts) > 0 {
		for _, art := range report.Artifacts {
			log.Printf("[PROCESSOR] 📎 Artifact [%s]:\n%s", art.Filename, art.Content)
		}
	}
	p.broadcastLog(fmt.Sprintf("📋 %s (%s) fait son rapport: %s", ag.ID[:8], ag.Role, truncate(report.Summary, 80)))

	// Journal: record report
	if p.journal != nil {
		var reportContent string
		reportContent = fmt.Sprintf("**Confiance : %.0f%%**\n\n%s", report.Confidence*100, report.Summary)
		if len(report.Artifacts) > 0 {
			reportContent += "\n\n**Artifacts :**\n"
			for _, art := range report.Artifacts {
				reportContent += fmt.Sprintf("- **%s** :\n```\n%s\n```\n", art.Filename, art.Content)
			}
		}
		p.journal.Record(ag.ID, string(ag.Role), ag.BubbleID, journal.EntryReport, reportContent)
	}

	// Report to parent
	p.reportToParent(ag, message.RptDone, map[string]interface{}{
		"result_summary":   report.Summary,
		"artifacts":        report.Artifacts,
		"confidence_score": report.Confidence,
		"agent_id":         ag.ID,
		"role":             ag.Role,
	})

	// Mark agent as done
	ag.Status = agent.StatusDead
	ag.UpdatedAt = time.Now()
	p.broadcastState(ag)

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, ag)
	}

	return nil
}

// killAgent terminates an agent and its children.
func (p *Processor) killAgent(ag *agent.Agent, reason string) error {
	log.Printf("[PROCESSOR] 💀 Killing agent %s: %s", ag.ID[:8], reason)
	p.broadcastLog(fmt.Sprintf("💀 Agent %s détruit: %s", ag.ID[:8], reason))

	// IMPORTANT: notify Postier/Architect that this child died (so parent doesn't wait forever)
	if ag.BubbleID != "" {
		p.reportToParent(ag, message.RptFail, map[string]interface{}{
			"reason":   reason,
			"agent_id": ag.ID,
			"role":     ag.Role,
		})
	} else {
		// If root agent is killed, still trigger final report to broadcast failure and save journal
		p.reportToParent(ag, message.RptFail, map[string]interface{}{
			"reason":   "Mission interrompue: " + reason,
			"agent_id": ag.ID,
			"status":   "MISSION FAILED",
		})
	}

	ag.Status = agent.StatusDead
	ag.UpdatedAt = time.Now()

	removed := p.registry.Remove(ag.ID)
	for _, id := range removed {
		p.budgetMgr.RemoveAgent(id)
		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventGraphUpdate,
			Payload: map[string]interface{}{
				"action":  "REMOVE_NODE",
				"agentId": id,
			},
		})
	}
	return nil
}

// reportToParent sends a report packet from an agent to the correct recipient.
// Routing logic:
//   - Worker with a PostierID → reports to its Postier
//   - Agent with a BubbleID but no PostierID → reports to the bubble's parent (small group)
//   - Root agent (no BubbleID) → broadcasts to frontend
func (p *Processor) reportToParent(ag *agent.Agent, pktType message.PacketType, body map[string]interface{}) {
	// Determine where to send the report
	targetID := ""
	if ag.PostierID != "" {
		// Worker with a Postier — report to the Postier
		targetID = ag.PostierID
	} else if ag.BubbleID != "" {
		// Small group or Postier reporting to parent — use BubbleID as target
		bubble := p.registry.GetBubble(ag.BubbleID)
		if bubble != nil && bubble.ParentAgentID != "" {
			targetID = bubble.ParentAgentID
		} else {
			// Fallback: BubbleID itself (old behavior)
			targetID = ag.BubbleID
		}
	}

	if targetID == "" {
		// Root agent — broadcast final result to frontend
		log.Printf("[PROCESSOR] 🏁 Root agent %s completed — broadcasting final result", ag.ID[:8])
		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventSystemAlert,
			Payload: map[string]interface{}{
				"message": fmt.Sprintf("Mission terminée par %s", ag.ID[:8]),
				"result":  body,
			},
		})

		if p.journal != nil {
			resultJSON, _ := json.MarshalIndent(body, "", "  ")
			p.journal.Record(ag.ID, string(ag.Role), ag.BubbleID, journal.EntryReport,
				fmt.Sprintf("**Rapport final :**\n```json\n%s\n```", string(resultJSON)))
			if path, err := p.journal.GenerateMarkdown(); err == nil {
				log.Printf("[PROCESSOR] 📓 Journal auto-généré: %s", path)
				p.broadcastLog(fmt.Sprintf("📓 Journal sauvegardé: %s", path))
			} else {
				log.Printf("[PROCESSOR] ❌ Erreur génération journal: %v", err)
			}
			p.journal.Reset()
		}

		if summary, ok := body["result_summary"].(string); ok {
			os.WriteFile("LIVRE_SF_COMPLET.md", []byte(summary), 0644)
		} else if result, ok := body["result"].(string); ok {
			os.WriteFile("LIVRE_SF_COMPLET.md", []byte(result), 0644)
		}
		return
	}

	pkt := message.Packet{
		Head: message.Header{
			ID:        uuid.New().String(),
			Timestamp: time.Now().Unix(),
			From:      ag.ID,
			To:        targetID,
			Type:      pktType,
		},
		Body: body,
	}
	p.dispatch.Enqueue(pkt)
}

// broadcastState sends an AGENT_STATE event to the frontend.
func (p *Processor) broadcastState(ag *agent.Agent) {
	p.wsHub.Broadcast(ws.Event{
		Type: ws.EventAgentState,
		Payload: map[string]interface{}{
			"agentId": ag.ID,
			"status":  ag.Status,
			"budget":  ag.Budget,
			"role":    ag.Role,
		},
	})
}

// broadcastLog sends a LOG_STREAM event to the frontend.
func (p *Processor) broadcastLog(msg string) {
	p.wsHub.Broadcast(ws.Event{
		Type: ws.EventLogStream,
		Payload: map[string]interface{}{
			"message": msg,
		},
	})
}

// normalizeAction maps various LLM action names to canonical actions.
// LLMs don't always use the exact names we expect.
func normalizeAction(action string) string {
	upper := strings.ToUpper(strings.TrimSpace(action))
	switch upper {
	// SPAWN variants
	case "SPAWN", "REQ_SPAWN", "DELEGATE", "CREATE", "SPLIT", "DECOMPOSE", "FORK":
		return "SPAWN"
	// WORK variants
	case "WORK", "EXECUTE", "DO", "PROCESS", "RUN", "GENERATE", "PRODUCE", "WRITE":
		return "WORK"
	// REPORT variants
	case "REPORT", "DONE", "COMPLETE", "FINISH", "SUMMARIZE", "SYNTHESIZE", "RPT_DONE", "RESULT":
		return "REPORT"
	default:
		return upper
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ExecSpawnPayload is a public test-helper that exercises the full SPAWN logic
// (Postier creation, bubble registration, worker spawning) without requiring an LLM call.
// It should only be used in tests.
func (p *Processor) ExecSpawnPayload(parent *agent.Agent, payloadBytes []byte) error {
	return p.handleSpawn(parent, payloadBytes)
}
