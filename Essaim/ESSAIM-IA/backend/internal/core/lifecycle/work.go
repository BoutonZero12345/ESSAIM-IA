package lifecycle

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/message"
)

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

	// Track worker completion and confidence
	p.stats.RecordWorkerDone()
	if work.Confidence > 0 {
		p.stats.SetFinalConfidence(work.Confidence)
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
