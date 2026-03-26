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
// When all children are done, the parent calls the LLM to synthesize results.
func (p *Processor) handleChildReport(parent *agent.Agent, pkt message.Packet) error {
	childID := pkt.Head.From
	log.Printf("[PROCESSOR] 📩 Parent %s received %s from child %s", parent.ID[:8], pkt.Head.Type, childID[:8])

	// Store child result in parent memory
	resultJSON, _ := json.MarshalIndent(pkt.Body, "", "  ")
	parent.Memory = append(parent.Memory, agent.Message{
		Role:    "user",
		Content: fmt.Sprintf("[CHILD_REPORT from %s (%s)]:\n%s", childID[:8], pkt.Head.Type, string(resultJSON)),
	})
	parent.UpdatedAt = time.Now()

	// Get bubble agents
	b := p.registry.GetBubble(parent.ID)
	var bubbleIDs []string
	if b != nil {
		bubbleIDs = b.AgentIDs
	}

	// --- POSTIER ROUTING (Horizontal) ---
	// Extract raw text to distribute
	var childResultText string
	var parsedRes map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &parsedRes); err == nil {
		if res, ok := parsedRes["result"].(string); ok {
			childResultText = res
		} else if sum, ok := parsedRes["result_summary"].(string); ok {
			childResultText = sum
		} else {
			childResultText = string(resultJSON)
		}
	} else {
		childResultText = string(resultJSON)
	}

	// Distribute to living siblings
	for _, cid := range bubbleIDs {
		if cid == childID || cid == parent.ID {
			continue
		}
		siblingNode := p.registry.Get(cid)
		if siblingNode != nil && siblingNode.Agent.Status != agent.StatusDead {
			siblingNode.Agent.Memory = append(siblingNode.Agent.Memory, agent.Message{
				Role:    "user",
				Content: fmt.Sprintf("==== CONTEXTE D'ENTRÉE SUPPLÉMENTAIRE ====\nUn agent de ton groupe vient de terminer son travail. Voici sa production pour t'aider dans ta tâche :\n%s", childResultText),
			})
			if p.repo != nil {
				p.repo.UpsertAgent(p.ctx, siblingNode.Agent)
			}
			log.Printf("[POSTIER] 📬 Context injected from %s to sibling %s", childID[:8], cid[:8])
		}
	}
	// ------------------------------------

	// Thread-safe trigger for Synthesis once all spawned children complete
	parent.SubtasksPending--

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, parent)
	}

	if parent.SubtasksPending > 0 {
		p.broadcastLog(fmt.Sprintf("⏳ %s attend d'autres sous-agents (%d restants)...", parent.ID[:8], parent.SubtasksPending))
		return nil
	}
	if parent.SubtasksPending < 0 {
		// Prevent cascading resumeur avalanche
		return nil
	}

	childResults := []string{}
	// Once all children are done, extract their ACTUAL final reports from the parent's memory
	for _, msg := range parent.Memory {
		if strings.HasPrefix(msg.Content, "[CHILD_REPORT") {
			parts := strings.SplitN(msg.Content, "]:\n", 2)
			if len(parts) == 2 {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(parts[1]), &parsed); err == nil {
					if res, ok := parsed["result"].(string); ok {
						childResults = append(childResults, res)
					} else if sum, ok := parsed["result_summary"].(string); ok {
						childResults = append(childResults, sum)
					} else {
						childResults = append(childResults, parts[1])
					}
				} else {
					childResults = append(childResults, parts[1])
				}
			}
		}
	}

	// Structure the synthesis based on child count
	numChildren := len(childResults)
	var finalSummary string

	if numChildren <= 3 {
		// 1-3 agents: No Resumeur, just concatenate.
		log.Printf("[PROCESSOR] 🔄 Group of %d agents (<=3) — no Resumeur, direct concatenation", numChildren)
		p.broadcastLog(fmt.Sprintf("🔄 Groupe de %d agents — concaténation directe sans Résumeur.", numChildren))
		for idx, res := range childResults {
			finalSummary += fmt.Sprintf("\n\n--- OUTPUT DE L'AGENT %d ---\n\n%s", idx+1, res)
		}
	} else if numChildren <= 7 {
		// 4-7 agents: 1 Resumeur
		log.Printf("[PROCESSOR] 🔄 Group of %d agents (4-7) — invoking 1 Resumeur", numChildren)
		p.broadcastLog(fmt.Sprintf("🔄 Groupe de %d agents — appel de 1 Résumeur...", numChildren))

		var allChildResultsStr string
		for idx, res := range childResults {
			allChildResultsStr += fmt.Sprintf("\n\n--- OUTPUT DE L'AGENT %d ---\n\n%s", idx+1, res)
		}

		finalSummary = p.callResumeur(parent, allChildResultsStr)
	} else {
		// 8-12 agents: 2 Resumeurs
		log.Printf("[PROCESSOR] 🔄 Group of %d agents (8-12) — invoking 2 Resumeurs", numChildren)
		p.broadcastLog(fmt.Sprintf("🔄 Groupe de %d agents — appel de 2 Résumeurs séquentiels...", numChildren))

		half := numChildren / 2
		var part1, part2 string
		for idx, res := range childResults[:half] {
			part1 += fmt.Sprintf("\n\n--- OUTPUT DE L'AGENT %d ---\n\n%s", idx+1, res)
		}
		for idx, res := range childResults[half:] {
			part2 += fmt.Sprintf("\n\n--- OUTPUT DE L'AGENT %d ---\n\n%s", half+idx+1, res)
		}

		sum1 := p.callResumeur(parent, part1)
		sum2 := p.callResumeur(parent, part2)

		finalSummary = fmt.Sprintf("=== SYNTHÈSE PARTIE 1 ===\n%s\n\n=== SYNTHÈSE PARTIE 2 ===\n%s", sum1, sum2)
	}

	// Then report to parent
	p.reportToParent(parent, message.RptDone, map[string]interface{}{
		"result_summary": finalSummary,
	})

	parent.Status = agent.StatusDead
	parent.UpdatedAt = time.Now()
	p.broadcastState(parent)
	return nil
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
func (p *Processor) handleSpawn(parent *agent.Agent, payload json.RawMessage) error {
	log.Printf("[PROCESSOR] 🔍 SPAWN payload raw: %s", string(payload))

	var spawn SpawnPayload
	subtasks := p.parseSubtasks(payload)

	// Enforce maximum of 12 sub-agents spawned by a single agent
	if len(subtasks) > 12 {
		log.Printf("[PROCESSOR] ⚠ Agent %s attempted to spawn %d children, capping at 12.", parent.ID[:8], len(subtasks))
		p.broadcastLog(fmt.Sprintf("⚠ %s tente de créer %d enfants (limite 12) - Troncature.", parent.ID[:8], len(subtasks)))
		subtasks = subtasks[:12]
	}

	spawn.Subtasks = subtasks

	log.Printf("[PROCESSOR] 🔀 Agent %s spawning %d children", parent.ID[:8], len(spawn.Subtasks))
	p.broadcastLog(fmt.Sprintf("🔀 %s crée %d sous-agents", parent.ID[:8], len(spawn.Subtasks)))

	// Journal: record spawn
	if p.journal != nil {
		var spawnInfo string
		for _, t := range spawn.Subtasks {
			spawnInfo += fmt.Sprintf("- **%s** : %s\n", t.Role, t.TaskDescription)
		}
		p.journal.Record(parent.ID, string(parent.Role), parent.BubbleID, journal.EntrySpawn, fmt.Sprintf("Création de %d sous-agents :\n%s", len(spawn.Subtasks), spawnInfo))
	}

	parent.Status = agent.StatusWaiting
	parent.UpdatedAt = time.Now()
	parent.SubtasksPending = len(spawn.Subtasks)
	p.broadcastState(parent)

	// Calculate fair budget fraction based on number of children
	numChildren := len(spawn.Subtasks)
	fairFraction := 0.8 / float64(numChildren) // 80% of budget split among children, 20% reserve
	if fairFraction > 0.5 {
		fairFraction = 0.5
	}
	if fairFraction < 0.001 {
		fairFraction = 0.001
	}

	for i, task := range spawn.Subtasks {
		fraction := task.BudgetFraction
		if fraction <= 0 || fraction > 0.8 {
			fraction = fairFraction
		}

		childBudget, err := p.budgetMgr.AllocateFromParent(parent.ID, fraction)
		if err != nil {
			log.Printf("[PROCESSOR] ⚠ Budget allocation failed for child %d: %v", i, err)
			p.broadcastLog(fmt.Sprintf("⚠ Budget insuffisant pour créer le sous-agent %d", i+1))
			parent.SubtasksPending--
			continue
		}

		role := agent.AgentRole(task.Role)
		if role == "" {
			role = agent.RoleWorker
		}

		child := &agent.Agent{
			ID:         uuid.New().String(),
			BubbleID:   parent.ID, // The parent's ID becomes the BubbleID for this group
			Role:       role,
			Status:     agent.StatusBorn,
			Budget:     childBudget,
			RetryCount: 0,
			Memory:     make([]agent.Message, 0),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := p.registry.Register(child); err != nil {
			log.Printf("[PROCESSOR] ❌ Failed to register child: %v", err)
			p.broadcastLog(fmt.Sprintf("❌ Échec enregistrement sous-agent: %v", err))
			parent.SubtasksPending--
			continue
		}

		if err := p.budgetMgr.RegisterAgent(child.ID, childBudget); err != nil {
			log.Printf("[PROCESSOR] ❌ Failed to register child budget: %v", err)
			continue
		}

		// The Registry handles adding the child to the parent's Bubble (ID = parent.ID)

		if p.repo != nil {
			p.repo.UpsertAgent(p.ctx, child)
		}

		// Broadcast new node to frontend
		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventGraphUpdate,
			Payload: map[string]interface{}{
				"action": "ADD_NODE",
				"agent":  child,
			},
		})

		log.Printf("[PROCESSOR] ✅ Spawned child %s (%s) for task: %s", child.ID[:8], child.Role, task.TaskDescription[:min(50, len(task.TaskDescription))])
		p.broadcastLog(fmt.Sprintf("✅ Sous-agent %s (%s) créé: %s", child.ID[:8], child.Role, truncate(task.TaskDescription, 60)))

		// Send the task to the child
		taskPkt := message.Packet{
			Head: message.Header{
				ID:        uuid.New().String(),
				Timestamp: time.Now().Unix(),
				From:      parent.ID,
				To:        child.ID,
				Type:      message.CmdTask,
			},
			Body: map[string]interface{}{
				"task_description": task.TaskDescription,
				"from_parent":      parent.ID,
				"parent_role":      string(parent.Role),
			},
		}
		p.dispatch.Enqueue(taskPkt)
	}

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, parent)
	}

	// FALLBACK: if no children were created (all budget allocations failed),
	// the agent does WORK itself instead of waiting forever
	bBubble := p.registry.GetBubble(parent.ID)
	numSpawned := 0
	if bBubble != nil {
		numSpawned = len(bBubble.AgentIDs)
	}
	if numSpawned == 0 {
		log.Printf("[PROCESSOR] ⚠ Agent %s SPAWN failed (0 children created) — falling back to WORK", parent.ID[:8])
		p.broadcastLog(fmt.Sprintf("⚠ %s n'a pu créer aucun sous-agent — fait le travail lui-même", parent.ID[:8]))

		parent.Status = agent.StatusWorking
		parent.UpdatedAt = time.Now()
		p.broadcastState(parent)

		// Combine all subtask descriptions into a single task
		var combinedTask string
		for _, task := range spawn.Subtasks {
			combinedTask += "- " + task.TaskDescription + "\n"
		}

		// Re-call LLM with a forced WORK prompt
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

		// Deduct tokens
		p.budgetMgr.Deduct(parent.ID, float64(resp.TokensUsed))
		parent.Budget, _ = p.budgetMgr.GetBudget(parent.ID)

		log.Printf("[PROCESSOR] ✅ Fallback WORK for %s: %d tokens", parent.ID[:8], resp.TokensUsed)

		var action LLMAction
		if err := json.Unmarshal([]byte(resp.RawJSON), &action); err == nil {
			return p.handleWork(parent, action.Payload)
		}
		// If parse fails, treat raw response as work result
		return p.handleWork(parent, json.RawMessage(fmt.Sprintf(`{"result": %s, "confidence": 0.7}`, resp.RawJSON)))
	}

	if p.repo != nil {
		p.repo.UpsertAgent(p.ctx, parent)
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

// reportToParent sends a report packet to the Bubble's Postier/Creator.
func (p *Processor) reportToParent(ag *agent.Agent, pktType message.PacketType, body map[string]interface{}) {
	if ag.BubbleID == "" {
		// Root agent — broadcast final result to frontend
		log.Printf("[PROCESSOR] 🏁 Root agent %s completed — broadcasting final result", ag.ID[:8])
		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventSystemAlert,
			Payload: map[string]interface{}{
				"message": fmt.Sprintf("Mission terminée par %s", ag.ID[:8]),
				"result":  body,
			},
		})

		// Auto-generate journal markdown
		if p.journal != nil {
			resultJSON, _ := json.MarshalIndent(body, "", "  ")
			p.journal.Record(ag.ID, string(ag.Role), ag.BubbleID, journal.EntryReport, fmt.Sprintf("**Rapport final :**\n```json\n%s\n```", string(resultJSON)))
			if path, err := p.journal.GenerateMarkdown(); err == nil {
				log.Printf("[PROCESSOR] 📓 Journal auto-généré: %s", path)
				p.broadcastLog(fmt.Sprintf("📓 Journal sauvegardé: %s", path))
			} else {
				log.Printf("[PROCESSOR] ❌ Erreur génération journal: %v", err)
			}
			p.journal.Reset()
		}

		// Also write the raw text to a dedicated file for massive texts
		if summary, ok := body["result_summary"].(string); ok {
			os.WriteFile("LIVRE_SF_COMPLET.md", []byte(summary), 0644)
			log.Printf("[PROCESSOR] 📖 Livre final exporté dans LIVRE_SF_COMPLET.md")
		} else if result, ok := body["result"].(string); ok {
			os.WriteFile("LIVRE_SF_COMPLET.md", []byte(result), 0644)
			log.Printf("[PROCESSOR] 📖 Livre final exporté dans LIVRE_SF_COMPLET.md")
		}

		return
	}

	pkt := message.Packet{
		Head: message.Header{
			ID:        uuid.New().String(),
			Timestamp: time.Now().Unix(),
			From:      ag.ID,
			To:        ag.BubbleID,
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
