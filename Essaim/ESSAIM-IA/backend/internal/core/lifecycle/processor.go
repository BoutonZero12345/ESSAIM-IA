package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"essaim-backend/internal/core/economy"
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
) *Processor {
	return &Processor{
		ctx:       ctx,
		registry:  registry,
		budgetMgr: budgetMgr,
		llm:       llmClient,
		repo:      repo,
		wsHub:     wsHub,
		dispatch:  dispatch,
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

	// Generate the system prompt
	systemPrompt := agent.GenerateSystemPrompt(ag)

	// Add task context to memory
	ag.Memory = append(ag.Memory, agent.Message{
		Role:    "user",
		Content: userMessage,
	})

	log.Printf("[PROCESSOR] 🤖 Calling LLM for agent %s (%s)...", ag.ID[:8], ag.Role)
	p.broadcastLog(fmt.Sprintf("🤖 Appel LLM pour %s (%s)...", ag.ID[:8], ag.Role))

	// Call the LLM
	resp, err := p.llm.Call(p.ctx, systemPrompt, userMessage)
	if err != nil {
		log.Printf("[PROCESSOR] ❌ LLM call failed for agent %s: %v", ag.ID[:8], err)
		p.broadcastLog(fmt.Sprintf("❌ Erreur LLM pour %s: %v", ag.ID[:8], err))

		// Send failure report to parent
		p.reportToParent(ag, message.RptFail, map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	log.Printf("[PROCESSOR] ✅ LLM response for %s: %d tokens, %dms", ag.ID[:8], resp.TokensUsed, resp.LatencyMs)
	log.Printf("[PROCESSOR] 📄 Raw LLM output for %s:\n%s", ag.ID[:8], resp.RawJSON)
	p.broadcastLog(fmt.Sprintf("✅ Réponse LLM pour %s: %d tokens, %dms", ag.ID[:8], resp.TokensUsed, resp.LatencyMs))

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
			ToAgentID:   ag.ParentID,
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

	// Check if ALL children are done
	allDone := true
	childResults := []string{}
	for _, cid := range parent.ChildrenIDs {
		childNode := p.registry.Get(cid)
		if childNode == nil {
			// Child was removed (dead) — considered done
			childResults = append(childResults, fmt.Sprintf("Agent %s: [terminated]", cid[:8]))
			continue
		}
		if childNode.Agent.Status != agent.StatusDead {
			allDone = false
			log.Printf("[PROCESSOR] ⏳ Parent %s still waiting for child %s (%s)", parent.ID[:8], cid[:8], childNode.Agent.Status)
		} else {
			childResults = append(childResults, fmt.Sprintf("Agent %s (%s): done", cid[:8], childNode.Agent.Role))
		}
	}

	if !allDone {
		p.broadcastLog(fmt.Sprintf("⏳ %s attend d'autres sous-agents...", parent.ID[:8]))
		return nil
	}

	// All children are done — synthesize results
	log.Printf("[PROCESSOR] 🔄 All children of %s are done — synthesizing results", parent.ID[:8])
	p.broadcastLog(fmt.Sprintf("🔄 Tous les sous-agents de %s ont terminé — synthèse en cours...", parent.ID[:8]))

	parent.Status = agent.StatusWorking
	parent.UpdatedAt = time.Now()
	p.broadcastState(parent)

	// Check LLM client
	if p.llm == nil {
		// No LLM — just pass through the raw child results
		p.reportToParent(parent, message.RptDone, map[string]interface{}{
			"result_summary": "Synthesis (no LLM available)",
			"child_results":  pkt.Body,
		})
		parent.Status = agent.StatusDead
		parent.UpdatedAt = time.Now()
		p.broadcastState(parent)
		return nil
	}

	// Build synthesis prompt
	synthesisPrompt := fmt.Sprintf(
		"Tes sous-agents ont terminé leur travail. Voici leurs rapports :\n\n%s\n\n"+
			"Synthétise ces résultats en un rapport final cohérent. "+
			"Réponds en JSON strict : {\"action\": \"REPORT\", \"payload\": {\"result_summary\": \"...\", \"artifacts\": [{\"filename\": \"...\", \"content\": \"...\"}], \"confidence_score\": 0.0-1.0}}",
		string(resultJSON),
	)

	systemPrompt := agent.GenerateSystemPrompt(parent)
	resp, err := p.llm.Call(p.ctx, systemPrompt, synthesisPrompt)
	if err != nil {
		log.Printf("[PROCESSOR] ❌ Synthesis LLM call failed for %s: %v", parent.ID[:8], err)
		// Report raw child results to parent's parent
		p.reportToParent(parent, message.RptDone, pkt.Body)
		parent.Status = agent.StatusDead
		parent.UpdatedAt = time.Now()
		p.broadcastState(parent)
		return err
	}

	log.Printf("[PROCESSOR] ✅ Synthesis for %s: %d tokens, %dms", parent.ID[:8], resp.TokensUsed, resp.LatencyMs)
	log.Printf("[PROCESSOR] 📄 Synthesis output:\n%s", resp.RawJSON)

	// Deduct tokens
	bankrupt, _ := p.budgetMgr.Deduct(parent.ID, float64(resp.TokensUsed))
	parent.Budget, _ = p.budgetMgr.GetBudget(parent.ID)

	if bankrupt {
		p.broadcastLog(fmt.Sprintf("💀 %s en faillite pendant la synthèse", parent.ID[:8]))
		return p.killAgent(parent, "budget épuisé pendant synthèse")
	}

	// Parse synthesis response
	var action LLMAction
	if err := json.Unmarshal([]byte(resp.RawJSON), &action); err != nil {
		log.Printf("[PROCESSOR] ⚠ Synthesis response not parseable, forwarding raw")
		p.reportToParent(parent, message.RptDone, map[string]interface{}{
			"result_summary": resp.RawJSON,
		})
	} else {
		switch action.Action {
		case "SPAWN":
			return p.handleSpawn(parent, action.Payload)
		case "REPORT":
			return p.handleReport(parent, action.Payload)
		default:
			return p.handleReport(parent, action.Payload)
		}
	}

	parent.Status = agent.StatusDead
	parent.UpdatedAt = time.Now()
	p.broadcastState(parent)
	return nil
}

// handleSpawn creates child agents from the LLM's SPAWN action.
func (p *Processor) handleSpawn(parent *agent.Agent, payload json.RawMessage) error {
	log.Printf("[PROCESSOR] 🔍 SPAWN payload raw: %s", string(payload))

	var spawn SpawnPayload
	subtasks := p.parseSubtasks(payload)
	spawn.Subtasks = subtasks

	log.Printf("[PROCESSOR] 🔀 Agent %s spawning %d children", parent.ID[:8], len(spawn.Subtasks))
	p.broadcastLog(fmt.Sprintf("🔀 %s crée %d sous-agents", parent.ID[:8], len(spawn.Subtasks)))

	parent.Status = agent.StatusWaiting
	parent.UpdatedAt = time.Now()
	p.broadcastState(parent)

	for i, task := range spawn.Subtasks {
		fraction := task.BudgetFraction
		if fraction <= 0 || fraction > 0.5 {
			fraction = 0.2
		}

		childBudget, err := p.budgetMgr.AllocateFromParent(parent.ID, fraction)
		if err != nil {
			log.Printf("[PROCESSOR] ⚠ Budget allocation failed for child %d: %v", i, err)
			p.broadcastLog(fmt.Sprintf("⚠ Budget insuffisant pour créer le sous-agent %d", i+1))
			continue
		}

		role := agent.AgentRole(task.Role)
		if role == "" {
			role = agent.RoleWorker
		}

		child := &agent.Agent{
			ID:          uuid.New().String(),
			ParentID:    parent.ID,
			Role:        role,
			Status:      agent.StatusBorn,
			Budget:      childBudget,
			Memory:      make([]agent.Message, 0),
			ChildrenIDs: make([]string, 0),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := p.registry.Register(child); err != nil {
			log.Printf("[PROCESSOR] ❌ Failed to register child: %v", err)
			p.broadcastLog(fmt.Sprintf("❌ Échec enregistrement sous-agent: %v", err))
			continue
		}

		if err := p.budgetMgr.RegisterAgent(child.ID, childBudget); err != nil {
			log.Printf("[PROCESSOR] ❌ Failed to register child budget: %v", err)
			continue
		}

		parent.ChildrenIDs = append(parent.ChildrenIDs, child.ID)

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
		for _, key := range []string{"subtasks", "tasks", "sub_tasks", "children", "agents"} {
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
	for _, key := range []string{"task_description", "description", "task", "instruction", "objective"} {
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

	// Report to parent
	p.reportToParent(ag, message.RptDone, map[string]interface{}{
		"result":     work.Result,
		"confidence": work.Confidence,
		"agent_id":   ag.ID,
		"role":       ag.Role,
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

	// IMPORTANT: notify parent that this child died (so parent doesn't wait forever)
	if ag.ParentID != "" {
		p.reportToParent(ag, message.RptFail, map[string]interface{}{
			"reason":   reason,
			"agent_id": ag.ID,
			"role":     ag.Role,
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

// reportToParent sends a report packet to the parent agent.
func (p *Processor) reportToParent(ag *agent.Agent, pktType message.PacketType, body map[string]interface{}) {
	if ag.ParentID == "" {
		// Root agent — broadcast final result to frontend
		log.Printf("[PROCESSOR] 🏁 Root agent %s completed — broadcasting final result", ag.ID[:8])
		p.wsHub.Broadcast(ws.Event{
			Type: ws.EventSystemAlert,
			Payload: map[string]interface{}{
				"message": fmt.Sprintf("Mission terminée par %s", ag.ID[:8]),
				"result":  body,
			},
		})
		return
	}

	pkt := message.Packet{
		Head: message.Header{
			ID:        uuid.New().String(),
			Timestamp: time.Now().Unix(),
			From:      ag.ID,
			To:        ag.ParentID,
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
