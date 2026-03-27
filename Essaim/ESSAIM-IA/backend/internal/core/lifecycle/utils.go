package lifecycle

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"essaim-backend/internal/core/evaluation"
	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/message"
	ws "essaim-backend/internal/infrastructure/websocket"
	"github.com/google/uuid"
)

// killAgent terminates an agent and its children.
func (p *Processor) killAgent(ag *agent.Agent, reason string) error {
	log.Printf("[PROCESSOR] 💀 Killing agent %s: %s", ag.ID[:8], reason)
	p.broadcastLog(fmt.Sprintf("💀 Agent %s détruit: %s", ag.ID[:8], reason))

	// Track failure
	p.stats.RecordWorkerFailed()

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

		// Generate the mission auto-bilan
		p.stats.Finish()
		if bilanPath, err := evaluation.WriteBilan(p.stats, "logs/bilans"); err == nil {
			log.Printf("[PROCESSOR] 📋 Bilan auto-généré: %s", bilanPath)
			p.broadcastLog(fmt.Sprintf("📋 Bilan de mission sauvegardé: %s", bilanPath))
		} else {
			log.Printf("[PROCESSOR] ⚠ Erreur génération bilan: %v", err)
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
