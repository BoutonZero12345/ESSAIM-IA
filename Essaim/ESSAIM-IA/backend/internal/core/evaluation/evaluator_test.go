package evaluation

import (
	"strings"
	"testing"
	"time"
)

// --- Tests for GenerateBilan() heuristics ---

func TestBilanAllSuccess(t *testing.T) {
	s := NewMissionStats("Rédige un poème en 4 strophes")
	s.RecordSpawn(4)
	s.RecordPostier()
	// Simulate 4 workers completing
	s.RecordWorkerDone()
	s.RecordWorkerDone()
	s.RecordWorkerDone()
	s.RecordWorkerDone()
	s.SetFinalConfidence(0.92)
	s.StartTime = time.Now().Add(-30 * time.Second)
	s.EndTime = time.Now()

	bilan := GenerateBilan(s)
	t.Logf("Generated bilan:\n%s", bilan)

	// Should contain positives
	if !strings.Contains(bilan, "✅") {
		t.Error("Bilan should contain a positives section")
	}
	// Should NOT mention worker failures
	if strings.Contains(bilan, "Taux d'échec") {
		t.Error("Perfect run bilan should not mention failure rate")
	}
	// Confidence should be mentioned
	if !strings.Contains(bilan, "92%") {
		t.Error("Final confidence (92%) should appear in the bilan")
	}
	// Should mention 100% success rate
	if !strings.Contains(bilan, "100%") {
		t.Error("Worker success rate (100%) should appear in the bilan")
	}
}

func TestBilanWithFallbacks(t *testing.T) {
	s := NewMissionStats("Analyse complexe en 10 parties")
	s.RecordSpawn(10)
	s.WorkersCompleted = 8
	s.WorkersFailed = 2
	s.FallbacksTriggered = 3
	s.FinalConfidence = 0.75
	s.EndTime = s.StartTime.Add(2 * time.Minute)

	bilan := GenerateBilan(s)

	// Should mention fallbacks in improvements section
	if !strings.Contains(bilan, "fallback") {
		t.Error("Bilan should mention fallbacks in the improvements section")
	}
	// Should suggest budget increase
	if !strings.Contains(bilan, "budget") {
		t.Error("Bilan should suggest budget increase for fallbacks")
	}
	// Should report worker failures
	if !strings.Contains(bilan, "échoué") {
		t.Error("Bilan should report worker failures")
	}
}

func TestBilanLowConfidence(t *testing.T) {
	s := NewMissionStats("Tâche risquée")
	s.RecordSpawn(2)
	s.WorkersCompleted = 2
	s.FinalConfidence = 0.45
	s.EndTime = s.StartTime.Add(15 * time.Second)

	bilan := GenerateBilan(s)

	// Should flag low confidence in improvements
	if !strings.Contains(bilan, "faible") {
		t.Error("Low confidence (45%) should trigger a quality warning")
	}
	if !strings.Contains(bilan, "45%") {
		t.Error("Exact confidence percentage should appear in the bilan")
	}
}

func TestBilanFormat_UnderOnePage(t *testing.T) {
	// Worst case: everything went wrong → most verbose bilan possible
	s := NewMissionStats("Mission de test maximale avec objectif très long pour tester la limite de taille du document de bilan généré automatiquement")
	s.RecordSpawn(12)
	s.RecordPostier()
	s.RecordResumeur()
	s.RecordFallback()
	s.RecordFallback()
	s.RecordRetry()
	s.RecordRetry()
	s.RecordRetry()
	s.RecordRetry()
	s.WorkersCompleted = 8
	s.WorkersFailed = 4
	s.FinalConfidence = 0.50
	s.EndTime = s.StartTime.Add(10 * time.Minute)

	bilan := GenerateBilan(s)
	lines := strings.Split(bilan, "\n")

	// Less than 1 page = less than ~60 lines (A4 ~55-60 lines of text)
	if len(lines) > 65 {
		t.Errorf("Bilan is too long: %d lines (max 65 for < 1 page). Content:\n%s", len(lines), bilan)
	}

	// Must have all 3 required sections
	if !strings.Contains(bilan, "✅ Ce qui a bien") {
		t.Error("Missing positives section header")
	}
	if !strings.Contains(bilan, "🔧 Axes d'amélioration") {
		t.Error("Missing improvements section header")
	}
	if !strings.Contains(bilan, "📊 Métriques") {
		t.Error("Missing metrics section header")
	}
}

func TestBilanNoRetries(t *testing.T) {
	s := NewMissionStats("Test simple")
	s.WorkersCompleted = 1
	s.FinalConfidence = 0.90
	s.EndTime = s.StartTime.Add(5 * time.Second)

	bilan := GenerateBilan(s)

	if !strings.Contains(bilan, "retry") {
		t.Error("Bilan should mention retry count (even when 0)")
	}
	// 0 retries → positive mention
	if !strings.Contains(bilan, "Aucun retry") {
		t.Error("0 retries should produce a positive mention")
	}
}

func TestBilanResumeurMentioned(t *testing.T) {
	s := NewMissionStats("Grande mission multi-agents")
	s.RecordSpawn(7)
	s.RecordPostier()
	s.RecordResumeur()
	s.WorkersCompleted = 7
	s.FinalConfidence = 0.88
	s.EndTime = s.StartTime.Add(4 * time.Minute)

	bilan := GenerateBilan(s)

	if !strings.Contains(bilan, "Résumeur") {
		t.Error("Bilan should mention the Résumeur when one was called")
	}
}
