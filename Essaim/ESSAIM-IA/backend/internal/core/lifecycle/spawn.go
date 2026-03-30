package lifecycle

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
	ws "essaim-backend/internal/infrastructure/websocket"

	"github.com/google/uuid"
)

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

	// Track total workers spawned
	p.stats.RecordSpawn(numWorkers)

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
				// Track Postier creation
				p.stats.RecordPostier()
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
		// Track the fallback (budget too tight for delegation)
		p.stats.RecordFallback()

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

// ExecSpawnPayload is a public test-helper that exercises the full SPAWN logic
// (Postier creation, bubble registration, worker spawning) without requiring an LLM call.
// It should only be used in tests.
func (p *Processor) ExecSpawnPayload(parent *agent.Agent, payloadBytes []byte) error {
	return p.handleSpawn(parent, payloadBytes)
}
