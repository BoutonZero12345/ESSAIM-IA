package lifecycle_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"essaim-backend/internal/core/economy"
	"essaim-backend/internal/core/lifecycle"
	"essaim-backend/internal/core/orchestrator"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
	ws "essaim-backend/internal/infrastructure/websocket"
)

// --- Test helpers with correct API signatures ---

func newTestRegistry() *graph.Registry {
	return graph.NewRegistry()
}

func newTestBudgetMgr() *economy.BudgetManager {
	return economy.NewBudgetManager(1_000_000.0) // 1M token global budget for tests
}

func newTestHub() *ws.Hub {
	return ws.NewHub() // No .Run() needed — Hub is lazy on broadcast
}

func newTestProcessor(registry *graph.Registry, budgetMgr *economy.BudgetManager) *lifecycle.Processor {
	hub := newTestHub()
	// Dispatcher needs registry, budgetMgr, and a handler function
	// We use a no-op handler — tests drive spawning directly, not via dispatcher
	dispatcher := orchestrator.NewDispatcher(registry, budgetMgr, func(pkt message.Packet) error {
		return nil // No-op: don't process packets in tests
	})
	return lifecycle.NewProcessor(context.Background(), registry, budgetMgr, nil, nil, hub, dispatcher, nil)
}

// === TEST 1: Bubble closes only when ALL workers have reported done ===

func TestBubble_ParentNotNotifiedBeforeAllWorkersDone(t *testing.T) {
	registry := newTestRegistry()

	bubbleID := "test-bubble-1"
	parentID := "parent-001"
	registry.RegisterBubble(bubbleID, parentID, 4)

	// 1/4
	done, results := registry.RecordWorkerDone(bubbleID, "res1")
	if done {
		t.Fatal("Bubble should not be complete after 1/4")
	}
	if results != nil {
		t.Fatal("Results should be nil when not done")
	}

	// 2/4 and 3/4
	registry.RecordWorkerDone(bubbleID, "res2")
	registry.RecordWorkerDone(bubbleID, "res3")

	b := registry.GetBubble(bubbleID)
	if b.IsComplete() {
		t.Fatal("Bubble should not be complete after 3/4 workers")
	}

	// 4/4 — now it should close
	done, results = registry.RecordWorkerDone(bubbleID, "res4")
	if !done {
		t.Fatal("Bubble SHOULD be complete after 4/4 workers")
	}
	if len(results) != 4 {
		t.Fatalf("Expected 4 results, got %d", len(results))
	}
}

// === TEST 2: Bubble size rules — correct Postier+Résumeur decisions ===

func TestBubbleSizes_PostierAndResumeurRules(t *testing.T) {
	tests := []struct {
		workers         int
		expectPostier   bool
		expectResumeurs int
	}{
		{1, false, 0},
		{2, false, 0},
		{3, true, 0},
		{4, true, 0},
		{5, true, 1},
		{6, true, 1},
		{8, true, 1},
		{9, true, 2},
		{12, true, 2},
	}

	for _, tt := range tests {
		gotPostier := graph.NeedsPostier(tt.workers)
		gotResumeurs := graph.ResumeurCount(tt.workers)

		if gotPostier != tt.expectPostier {
			t.Errorf("NeedsPostier(%d): got %v, want %v", tt.workers, gotPostier, tt.expectPostier)
		}
		if gotResumeurs != tt.expectResumeurs {
			t.Errorf("ResumeurCount(%d): got %d, want %d", tt.workers, gotResumeurs, tt.expectResumeurs)
		}
	}
}

// === TEST 3: Thread-safety — concurrent workers reporting to same bubble ===

func TestBubble_ConcurrentWorkersDone(t *testing.T) {
	registry := newTestRegistry()

	bubbleID := "concurrent-bubble"
	parentID := "parent-concurrent"
	numWorkers := 8
	registry.RegisterBubble(bubbleID, parentID, numWorkers)

	var wg sync.WaitGroup
	completions := make(chan bool, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done, _ := registry.RecordWorkerDone(bubbleID, "worker-result")
			completions <- done
		}()
	}

	wg.Wait()
	close(completions)

	doneCount := 0
	for done := range completions {
		if done {
			doneCount++
		}
	}

	if doneCount == 0 {
		t.Error("Bubble completion was never triggered")
	}

	b := registry.GetBubble(bubbleID)
	if !b.IsComplete() {
		t.Errorf("Final bubble state: WorkersDone=%d, WorkerCount=%d — should be complete",
			b.WorkersDone, b.WorkerCount)
	}
}

// === TEST 4: Processor spawn creates Postier for N>=3 and routes workers correctly ===

func TestProcessor_SpawnCreatesPostierForLargeGroups(t *testing.T) {
	registry := newTestRegistry()
	budgetMgr := newTestBudgetMgr()
	proc := newTestProcessor(registry, budgetMgr)

	parentID := "parent-spawn-test"
	budgetMgr.RegisterAgent(parentID, 100000.0)

	parent := &agent.Agent{
		ID:        parentID,
		Role:      agent.RoleArchitect,
		Status:    agent.StatusWorking,
		Budget:    100000.0,
		Memory:    make([]agent.Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	registry.Register(parent)

	// SPAWN payload with 5 workers
	spawnPayload := map[string]interface{}{
		"subtasks": []map[string]interface{}{
			{"role": "WORKER", "task_description": "Rédige la section 1"},
			{"role": "WORKER", "task_description": "Rédige la section 2"},
			{"role": "WORKER", "task_description": "Rédige la section 3"},
			{"role": "WORKER", "task_description": "Rédige la section 4"},
			{"role": "WORKER", "task_description": "Rédige la section 5"},
		},
	}
	payloadBytes, _ := json.Marshal(spawnPayload)

	err := proc.ExecSpawnPayload(parent, payloadBytes)
	if err != nil {
		t.Fatalf("ExecSpawnPayload failed: %v", err)
	}

	allNodes := registry.AllNodes()
	postierCount := 0
	workerCount := 0
	for _, n := range allNodes {
		if n.Agent.ID == parentID {
			continue
		}
		if n.Agent.Role == agent.RolePostier {
			postierCount++
		}
		if n.Agent.Role == agent.RoleWorker {
			workerCount++
		}
	}

	if postierCount != 1 {
		t.Errorf("Expected 1 Postier for 5 workers, got %d", postierCount)
	}
	if workerCount != 5 {
		t.Errorf("Expected 5 Workers, got %d", workerCount)
	}

	// Parent should wait for exactly 1 result (from the Postier)
	parentNode := registry.Get(parentID)
	if parentNode.Agent.SubtasksPending != 1 {
		t.Errorf("Parent SubtasksPending = %d, want 1 (waiting for Postier)", parentNode.Agent.SubtasksPending)
	}

	// Workers should have PostierID set, not empty
	for _, n := range allNodes {
		if n.Agent.Role == agent.RoleWorker {
			if n.Agent.PostierID == "" {
				t.Errorf("Worker %s should have a PostierID set", n.Agent.ID[:8])
			}
		}
	}

	// Workers should all share the same bubbleID (which is NOT the parent's ID)
	var bubbleID string
	for _, n := range allNodes {
		if n.Agent.Role == agent.RoleWorker {
			if bubbleID == "" {
				bubbleID = n.Agent.BubbleID
			} else if n.Agent.BubbleID != bubbleID {
				t.Errorf("Workers have inconsistent BubbleIDs: %s vs %s", bubbleID, n.Agent.BubbleID)
			}
			if n.Agent.BubbleID == parentID {
				t.Errorf("Worker BubbleID should NOT equal parent.ID (old behavior). Got %s == %s", n.Agent.BubbleID, parentID)
			}
		}
	}
}

// === TEST 5: Small group (2 workers) — no Postier created ===

func TestProcessor_SpawnNoPostierForSmallGroups(t *testing.T) {
	registry := newTestRegistry()
	budgetMgr := newTestBudgetMgr()
	proc := newTestProcessor(registry, budgetMgr)

	parentID := "small-group-parent"
	budgetMgr.RegisterAgent(parentID, 50000.0)

	parent := &agent.Agent{
		ID:        parentID,
		Role:      agent.RoleManager,
		Status:    agent.StatusWorking,
		Budget:    50000.0,
		Memory:    make([]agent.Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	registry.Register(parent)

	spawnPayload := map[string]interface{}{
		"subtasks": []map[string]interface{}{
			{"role": "WORKER", "task_description": "Tâche A"},
			{"role": "WORKER", "task_description": "Tâche B"},
		},
	}
	payloadBytes, _ := json.Marshal(spawnPayload)

	err := proc.ExecSpawnPayload(parent, payloadBytes)
	if err != nil {
		t.Fatalf("ExecSpawnPayload failed: %v", err)
	}

	allNodes := registry.AllNodes()
	postierCount := 0
	for _, n := range allNodes {
		if n.Agent.Role == agent.RolePostier {
			postierCount++
		}
	}

	if postierCount != 0 {
		t.Errorf("Expected 0 Postier for 2 workers, got %d", postierCount)
	}

	// Parent should wait for 2 individual reports (no Postier)
	parentNode := registry.Get(parentID)
	if parentNode.Agent.SubtasksPending != 2 {
		t.Errorf("Parent SubtasksPending = %d, want 2 (no Postier — direct parent wait)", parentNode.Agent.SubtasksPending)
	}

	// Workers should have PostierID = "" (no Postier)
	for _, n := range allNodes {
		if n.Agent.Role == agent.RoleWorker {
			if n.Agent.PostierID != "" {
				t.Errorf("Worker %s should have empty PostierID for 2-worker group, got %s", n.Agent.ID[:8], n.Agent.PostierID)
			}
		}
	}
}
