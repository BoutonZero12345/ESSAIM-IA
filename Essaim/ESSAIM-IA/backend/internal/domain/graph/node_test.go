package graph

import (
	"testing"

	"essaim-backend/internal/domain/agent"
)

// --- Tests for Bubble size rules ---

func TestNeedsPostier(t *testing.T) {
	tests := []struct {
		workers int
		want    bool
	}{
		{1, false},
		{2, false},
		{3, true},
		{4, true},
		{5, true},
		{12, true},
	}
	for _, tt := range tests {
		got := NeedsPostier(tt.workers)
		if got != tt.want {
			t.Errorf("NeedsPostier(%d) = %v, want %v", tt.workers, got, tt.want)
		}
	}
}

func TestResumeurCount(t *testing.T) {
	tests := []struct {
		workers int
		want    int
	}{
		{2, 0},  // No Postier, no Résumeur
		{3, 0},  // 1 Postier, 0 Résumeur
		{4, 0},  // 1 Postier, 0 Résumeur
		{5, 1},  // 1 Postier, 1 Résumeur
		{6, 1},
		{7, 1},
		{8, 1},  // 1 Postier, 1 Résumeur (boundary)
		{9, 2},  // 1 Postier, 2 Résumeurs
		{10, 2},
		{12, 2},
	}
	for _, tt := range tests {
		got := ResumeurCount(tt.workers)
		if got != tt.want {
			t.Errorf("ResumeurCount(%d) = %d, want %d", tt.workers, got, tt.want)
		}
	}
}

// --- Tests for Bubble closed-group semantics ---

func TestBubbleIsCompleteRequiresNonZeroWorkerCount(t *testing.T) {
	b := &Bubble{WorkerCount: 0, WorkersDone: 0}
	if b.IsComplete() {
		t.Error("Empty bubble (WorkerCount=0) should not be complete")
	}
}

func TestBubbleIsCompleteAfterAllWorkersDone(t *testing.T) {
	b := &Bubble{WorkerCount: 4, WorkersDone: 0}
	if b.IsComplete() {
		t.Error("Bubble with 0/4 done should not be complete")
	}
	b.WorkersDone = 3
	if b.IsComplete() {
		t.Error("Bubble with 3/4 done should not be complete")
	}
	b.WorkersDone = 4
	if !b.IsComplete() {
		t.Error("Bubble with 4/4 done SHOULD be complete")
	}
}

// --- Tests for Registry Bubble management ---

func TestRegisterBubble(t *testing.T) {
	r := NewRegistry()
	r.RegisterBubble("bubble-1", "parent-agent", 5)

	b := r.GetBubble("bubble-1")
	if b == nil {
		t.Fatal("BubbleID 'bubble-1' should be registered")
	}
	if b.ParentAgentID != "parent-agent" {
		t.Errorf("ParentAgentID = %q, want 'parent-agent'", b.ParentAgentID)
	}
	if b.WorkerCount != 5 {
		t.Errorf("WorkerCount = %d, want 5", b.WorkerCount)
	}
	if b.WorkersDone != 0 {
		t.Errorf("WorkersDone should start at 0, got %d", b.WorkersDone)
	}
}

func TestRecordWorkerDone_NotYetComplete(t *testing.T) {
	r := NewRegistry()
	r.RegisterBubble("b1", "parent", 3)

	done, results := r.RecordWorkerDone("b1", "result1")
	if done {
		t.Error("Bubble should NOT be complete after 1/3 workers")
	}
	if results != nil {
		t.Error("Results should be nil when bubble is not complete")
	}

	done, results = r.RecordWorkerDone("b1", "result2")
	if done {
		t.Error("Bubble should NOT be complete after 2/3 workers")
	}
	if results != nil {
		t.Error("Results should be nil when bubble is not complete")
	}
}

func TestRecordWorkerDone_Complete(t *testing.T) {
	r := NewRegistry()
	r.RegisterBubble("b2", "parent-99", 3)

	r.RecordWorkerDone("b2", "res-A")
	r.RecordWorkerDone("b2", "res-B")
	done, results := r.RecordWorkerDone("b2", "res-C")

	if !done {
		t.Fatal("Bubble SHOULD be complete after 3/3 workers")
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Results must contain all 3
	expected := map[string]bool{"res-A": false, "res-B": false, "res-C": false}
	for _, r := range results {
		expected[r] = true
	}
	for k, found := range expected {
		if !found {
			t.Errorf("Expected result %q not found in collected results", k)
		}
	}
}

func TestRecordWorkerDone_UnknownBubble(t *testing.T) {
	r := NewRegistry()
	done, results := r.RecordWorkerDone("nonexistent-bubble", "some-result")
	if done {
		t.Error("Unknown bubble should return done=false")
	}
	if results != nil {
		t.Error("Unknown bubble should return nil results")
	}
}

func TestRegisterBubble_PostierID(t *testing.T) {
	r := NewRegistry()
	r.RegisterBubble("b3", "parent-X", 4)

	// Simulate registering a Postier agent
	postier := &agent.Agent{
		ID:       "postier-1",
		BubbleID: "b3",
		Role:     agent.RolePostier,
	}
	r.Register(postier)

	b := r.GetBubble("b3")
	if b == nil {
		t.Fatal("Bubble b3 not found")
	}
	if b.PostierID != "postier-1" {
		t.Errorf("PostierID = %q, want 'postier-1'", b.PostierID)
	}
}

func TestRecordWorkerDone_ExactCountEnforcesOnce(t *testing.T) {
	// Ensure a bubble that said "3 workers" marks complete at the 3rd, not the 4th
	r := NewRegistry()
	r.RegisterBubble("b4", "parent", 3)

	r.RecordWorkerDone("b4", "res1")
	r.RecordWorkerDone("b4", "res2")
	done, _ := r.RecordWorkerDone("b4", "res3")
	if !done {
		t.Error("Should be complete at exactly worker 3")
	}

	// A 4th call should still return done=true (already complete)
	done, results := r.RecordWorkerDone("b4", "res4")
	if !done {
		t.Error("Should stay done after 4th call (WorkersDone=4 >= WorkerCount=3)")
	}
	if len(results) != 4 {
		t.Errorf("Expected 4 results collected, got %d", len(results))
	}
}
