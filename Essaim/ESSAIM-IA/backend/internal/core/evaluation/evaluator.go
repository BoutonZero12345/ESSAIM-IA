package evaluation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MissionStats collects all metrics during a mission's lifecycle.
// It is incremented concurrently by the Processor and read once at the end.
type MissionStats struct {
	mu sync.Mutex

	Objective    string
	StartTime    time.Time
	EndTime      time.Time
	Finished     bool

	AgentsSpawned      int
	PostiersCreated    int
	ResumeursCalled    int
	WorkersCompleted   int
	WorkersFailed      int
	FallbacksTriggered int // agents who had to WORK instead of SPAWN (budget too tight)
	RetryCount         int
	BubbleCount        int // number of distinct bubbles registered
	FinalConfidence    float64
}

// NewMissionStats creates a fresh stats tracker for a new mission.
func NewMissionStats(objective string) *MissionStats {
	return &MissionStats{
		Objective: objective,
		StartTime: time.Now(),
	}
}

// --- Thread-safe increment helpers ---

func (s *MissionStats) RecordSpawn(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AgentsSpawned += n
}

func (s *MissionStats) RecordPostier() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PostiersCreated++
	s.BubbleCount++
}

func (s *MissionStats) RecordBubble() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BubbleCount++
}

func (s *MissionStats) RecordResumeur() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResumeursCalled++
}

func (s *MissionStats) RecordWorkerDone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WorkersCompleted++
}

func (s *MissionStats) RecordWorkerFailed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WorkersFailed++
}

func (s *MissionStats) RecordFallback() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FallbacksTriggered++
}

func (s *MissionStats) RecordRetry() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RetryCount++
}

func (s *MissionStats) SetFinalConfidence(confidence float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FinalConfidence = confidence
}

func (s *MissionStats) Finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EndTime = time.Now()
	s.Finished = true
}

// Duration returns the total mission duration. Safe to call after Finish().
func (s *MissionStats) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// GenerateBilan produces the mission report markdown. Less than 1 page.
func GenerateBilan(s *MissionStats) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	totalWorkers := s.WorkersCompleted + s.WorkersFailed
	successRate := 0
	if totalWorkers > 0 {
		successRate = int(float64(s.WorkersCompleted) / float64(totalWorkers) * 100)
	}

	dur := s.EndTime.Sub(s.StartTime)
	if s.EndTime.IsZero() {
		dur = time.Since(s.StartTime)
	}

	// --- Evaluate what went well ---
	var positives []string
	var improvements []string

	// Worker success rate
	if totalWorkers == 0 {
		positives = append(positives, "Mission mono-agent (pas de délégation) — exécution directe.")
	} else if successRate == 100 {
		positives = append(positives, fmt.Sprintf("%d workers complétés sans aucun échec (taux 100%%)", s.WorkersCompleted))
	} else if successRate >= 75 {
		positives = append(positives, fmt.Sprintf("%d/%d workers complétés (taux %d%%)", s.WorkersCompleted, totalWorkers, successRate))
	} else {
		improvements = append(improvements, fmt.Sprintf("Taux d'échec élevé : %d/%d workers ont échoué — vérifier le budget ou la complexité des tâches", s.WorkersFailed, totalWorkers))
	}

	// Bubble system
	if s.PostiersCreated > 0 {
		positives = append(positives, fmt.Sprintf("Système de bulles actif : %d Postier(s) créé(s) — messages correctement routés", s.PostiersCreated))
	} else if s.AgentsSpawned >= 3 {
		improvements = append(improvements, "Aucun Postier créé malgré un groupe de 3+ agents — vérifier la logique NeedsPostier()")
	}

	// Résumeurs
	if s.ResumeursCalled > 0 {
		positives = append(positives, fmt.Sprintf("%d Résumeur(s) appelé(s) — fenêtre de contexte compressée", s.ResumeursCalled))
	}

	// Final confidence
	if s.FinalConfidence >= 0.85 {
		positives = append(positives, fmt.Sprintf("Confiance finale élevée : %.0f%%", s.FinalConfidence*100))
	} else if s.FinalConfidence >= 0.70 {
		positives = append(positives, fmt.Sprintf("Confiance finale acceptable : %.0f%%", s.FinalConfidence*100))
	} else if s.FinalConfidence > 0 {
		improvements = append(improvements, fmt.Sprintf("Confiance finale faible (%.0f%%) — le résultat peut manquer de qualité", s.FinalConfidence*100))
	}

	// Fallbacks (budget)
	if s.FallbacksTriggered == 0 {
		positives = append(positives, "Aucun fallback budgétaire — le budget était suffisant pour déléguer")
	} else {
		improvements = append(improvements, fmt.Sprintf(
			"%d fallback(s) déclenché(s) — budget trop serré : augmenter de ~%d%% ou réduire le nombre de sous-agents",
			s.FallbacksTriggered,
			s.FallbacksTriggered*20,
		))
	}

	// Retries
	if s.RetryCount == 0 {
		positives = append(positives, "Aucun retry LLM — le modèle a répondu correctement du premier coup")
	} else if s.RetryCount <= 2 {
		improvements = append(improvements, fmt.Sprintf("%d retry(s) LLM — le modèle a eu des difficultés à respecter le format JSON", s.RetryCount))
	} else {
		improvements = append(improvements, fmt.Sprintf("%d retries LLM — instabilité du modèle ou format de prompt à revoir", s.RetryCount))
	}

	// --- Build the markdown ---
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Bilan de Mission — %s\n\n", s.StartTime.Format("2006-01-02 15:04")))

	// Truncate objective to 80 chars for readability
	objective := s.Objective
	if len(objective) > 80 {
		objective = objective[:77] + "..."
	}
	sb.WriteString(fmt.Sprintf("**Objectif :** %s  \n", objective))
	sb.WriteString(fmt.Sprintf("**Durée :** %s\n\n", formatDuration(dur)))

	// Positives section
	sb.WriteString("## ✅ Ce qui a bien fonctionné\n")
	if len(positives) == 0 {
		sb.WriteString("- *(aucun succès notable enregistré)*\n")
	} else {
		for _, p := range positives {
			sb.WriteString(fmt.Sprintf("- %s\n", p))
		}
	}

	sb.WriteString("\n")

	// Improvements section
	sb.WriteString("## 🔧 Axes d'amélioration\n")
	if len(improvements) == 0 {
		sb.WriteString("- *(aucun problème détecté — mission nominale)*\n")
	} else {
		for _, imp := range improvements {
			sb.WriteString(fmt.Sprintf("- %s\n", imp))
		}
	}

	sb.WriteString("\n")

	// Metrics table
	sb.WriteString("## 📊 Métriques\n\n")
	sb.WriteString("| Métrique | Valeur |\n|---|---|\n")
	sb.WriteString(fmt.Sprintf("| Agents créés | %d |\n", s.AgentsSpawned+s.PostiersCreated))
	sb.WriteString(fmt.Sprintf("| Postiers | %d |\n", s.PostiersCreated))
	sb.WriteString(fmt.Sprintf("| Résumeurs | %d |\n", s.ResumeursCalled))
	sb.WriteString(fmt.Sprintf("| Workers OK | %d |\n", s.WorkersCompleted))
	sb.WriteString(fmt.Sprintf("| Workers échoués | %d |\n", s.WorkersFailed))
	sb.WriteString(fmt.Sprintf("| Fallbacks budget | %d |\n", s.FallbacksTriggered))
	sb.WriteString(fmt.Sprintf("| Retries LLM | %d |\n", s.RetryCount))
	if s.FinalConfidence > 0 {
		sb.WriteString(fmt.Sprintf("| Confiance finale | %.0f%% |\n", s.FinalConfidence*100))
	} else {
		sb.WriteString("| Confiance finale | N/A |\n")
	}

	return sb.String()
}

// WriteBilan writes the bilan markdown to a file in the given directory.
// Returns the path of the generated file.
func WriteBilan(s *MissionStats, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create bilan directory: %w", err)
	}

	content := GenerateBilan(s)
	filename := fmt.Sprintf("BILAN_%s.md", s.StartTime.Format("20060102_150405"))
	path := filepath.Join(outputDir, filename)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("cannot write bilan file: %w", err)
	}

	return path, nil
}

// formatDuration formats a duration as "Xm Ys" for human readability.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m == 0 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}
