package lifecycle

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/message"
	ws "essaim-backend/internal/infrastructure/websocket"

	"github.com/google/uuid"
)

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

	// Track Resumeur invocation
	p.stats.RecordResumeur()

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
