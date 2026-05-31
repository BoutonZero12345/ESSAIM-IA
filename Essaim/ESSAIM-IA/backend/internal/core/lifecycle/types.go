package lifecycle

import "encoding/json"

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
