package journal

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// EntryType defines what kind of journal entry this is.
type EntryType string

const (
	EntryTaskReceived EntryType = "TASK_RECEIVED"
	EntryLLMResponse  EntryType = "LLM_RESPONSE"
	EntrySpawn        EntryType = "SPAWN"
	EntryWork         EntryType = "WORK"
	EntryReport       EntryType = "REPORT"
	EntrySynthesis    EntryType = "SYNTHESIS"
	EntryFallback     EntryType = "FALLBACK_WORK"
	EntryError        EntryType = "ERROR"
)

// Entry is a single journal interaction for an agent.
type Entry struct {
	Timestamp time.Time
	AgentID   string
	AgentRole string
	ParentID  string
	Type      EntryType
	Content   string
}

// Journal collects all agent interactions during a mission and auto-generates markdown.
type Journal struct {
	mu        sync.Mutex
	entries   []Entry
	objective string
	startTime time.Time
	outputDir string
}

// New creates a new Journal that writes to the given output directory.
func New(outputDir string) *Journal {
	// Ensure output directory exists
	os.MkdirAll(outputDir, 0755)
	return &Journal{
		entries:   make([]Entry, 0, 100),
		outputDir: outputDir,
	}
}

// SetObjective records the mission objective (called at mission start).
func (j *Journal) SetObjective(objective string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.objective = objective
	j.startTime = time.Now()
}

// Record adds an entry to the journal. Thread-safe.
func (j *Journal) Record(agentID, agentRole, parentID string, entryType EntryType, content string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = append(j.entries, Entry{
		Timestamp: time.Now(),
		AgentID:   agentID,
		AgentRole: agentRole,
		ParentID:  parentID,
		Type:      entryType,
		Content:   content,
	})
}

// GenerateMarkdown builds the markdown file and writes it to disk.
// Returns the path to the generated file.
func (j *Journal) GenerateMarkdown() (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.entries) == 0 {
		return "", fmt.Errorf("no entries to generate")
	}

	// Build agent map: agentID → {role, parentID, entries}
	type agentInfo struct {
		role     string
		parentID string
		entries  []Entry
	}
	agents := make(map[string]*agentInfo)
	// Preserve order of first appearance
	agentOrder := make([]string, 0)

	for _, e := range j.entries {
		if _, exists := agents[e.AgentID]; !exists {
			agents[e.AgentID] = &agentInfo{
				role:     e.AgentRole,
				parentID: e.ParentID,
				entries:  make([]Entry, 0),
			}
			agentOrder = append(agentOrder, e.AgentID)
		}
		agents[e.AgentID].entries = append(agents[e.AgentID].entries, e)
	}

	// Build tree: find root, then BFS order
	var rootID string
	childrenMap := make(map[string][]string)
	for id, info := range agents {
		if info.parentID == "" {
			rootID = id
		} else {
			childrenMap[info.parentID] = append(childrenMap[info.parentID], id)
		}
	}

	// Sort children by first appearance
	for _, children := range childrenMap {
		sort.Slice(children, func(i, j int) bool {
			idxI := indexOf(agentOrder, children[i])
			idxJ := indexOf(agentOrder, children[j])
			return idxI < idxJ
		})
	}

	// BFS to get hierarchical order
	orderedAgents := make([]string, 0, len(agents))
	depthMap := make(map[string]int)
	if rootID != "" {
		queue := []string{rootID}
		depthMap[rootID] = 0
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			orderedAgents = append(orderedAgents, id)
			for _, childID := range childrenMap[id] {
				depthMap[childID] = depthMap[id] + 1
				queue = append(queue, childID)
			}
		}
	}
	// Add any agents not in the tree (orphans)
	for _, id := range agentOrder {
		if _, found := depthMap[id]; !found {
			orderedAgents = append(orderedAgents, id)
			depthMap[id] = 0
		}
	}

	// Generate markdown
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# 🐝 ESSAIM Mission — %s\n\n", j.startTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Objectif :** %s\n\n", j.objective))
	sb.WriteString(fmt.Sprintf("**Agents créés :** %d\n\n", len(agents)))
	sb.WriteString("---\n\n")

	// Table of contents
	sb.WriteString("## 📋 Sommaire\n\n")
	for _, id := range orderedAgents {
		info := agents[id]
		indent := strings.Repeat("  ", depthMap[id])
		shortID := shortID(id)
		sb.WriteString(fmt.Sprintf("%s- [%s (%s)](#agent-%s)\n", indent, info.role, shortID, shortID))
	}
	sb.WriteString("\n---\n\n")

	// Each agent section
	for _, id := range orderedAgents {
		info := agents[id]
		shortAgentID := shortID(id)
		depth := depthMap[id]

		// Header with depth indicator
		depthIndicator := strings.Repeat("│  ", depth)
		if depth > 0 {
			depthIndicator = strings.Repeat("│  ", depth-1) + "├─ "
		}

		sb.WriteString(fmt.Sprintf("## <a id=\"agent-%s\"></a>%s%s `%s`\n\n", shortAgentID, depthIndicator, info.role, shortAgentID))

		if info.parentID != "" {
			sb.WriteString(fmt.Sprintf("*Parent : %s*\n\n", shortID(info.parentID)))
		}

		for _, entry := range info.entries {
			ts := entry.Timestamp.Format("15:04:05")

			switch entry.Type {
			case EntryTaskReceived:
				sb.WriteString(fmt.Sprintf("### 📨 Tâche reçue (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", entry.Content))

			case EntryLLMResponse:
				sb.WriteString(fmt.Sprintf("### 🤖 Réponse LLM (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", entry.Content))

			case EntrySpawn:
				sb.WriteString(fmt.Sprintf("### 🔀 Délégation (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("%s\n\n", entry.Content))

			case EntryWork:
				sb.WriteString(fmt.Sprintf("### 📝 Contenu produit (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("%s\n\n", entry.Content))

			case EntryReport:
				sb.WriteString(fmt.Sprintf("### 📋 Rapport final (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("%s\n\n", entry.Content))

			case EntrySynthesis:
				sb.WriteString(fmt.Sprintf("### 🔄 Synthèse (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("%s\n\n", entry.Content))

			case EntryFallback:
				sb.WriteString(fmt.Sprintf("### ⚠ Fallback WORK (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("%s\n\n", entry.Content))

			case EntryError:
				sb.WriteString(fmt.Sprintf("### ❌ Erreur (%s)\n\n", ts))
				sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", entry.Content))
			}
		}

		sb.WriteString("---\n\n")
	}

	// Footer
	duration := time.Since(j.startTime)
	sb.WriteString("## 📊 Statistiques\n\n")
	sb.WriteString(fmt.Sprintf("- **Durée totale :** %s\n", duration.Round(time.Second)))
	sb.WriteString(fmt.Sprintf("- **Nombre d'agents :** %d\n", len(agents)))

	// Count by type
	typeCounts := make(map[EntryType]int)
	for _, e := range j.entries {
		typeCounts[e.Type]++
	}
	sb.WriteString(fmt.Sprintf("- **Appels LLM :** %d\n", typeCounts[EntryLLMResponse]))
	sb.WriteString(fmt.Sprintf("- **WORK produits :** %d\n", typeCounts[EntryWork]))
	sb.WriteString(fmt.Sprintf("- **SPAWN délégations :** %d\n", typeCounts[EntrySpawn]))
	sb.WriteString(fmt.Sprintf("- **Synthèses :** %d\n", typeCounts[EntrySynthesis]))
	sb.WriteString(fmt.Sprintf("- **Erreurs :** %d\n", typeCounts[EntryError]))

	// Write file
	filename := fmt.Sprintf("mission_%s.md", j.startTime.Format("2006-01-02_15-04-05"))
	filePath := filepath.Join(j.outputDir, filename)

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write journal: %w", err)
	}

	log.Printf("[JOURNAL] 📓 Journal sauvegardé: %s (%d entrées, %d agents)", filePath, len(j.entries), len(agents))
	return filePath, nil
}

// Reset clears the journal for the next mission.
func (j *Journal) Reset() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = make([]Entry, 0, 100)
	j.objective = ""
	j.startTime = time.Time{}
}

// --- Helpers ---

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return len(slice)
}
