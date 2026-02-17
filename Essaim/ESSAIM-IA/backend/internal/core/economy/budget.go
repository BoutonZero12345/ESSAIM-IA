package economy

import (
	"fmt"
	"sync"
)

// BudgetManager implements the "Central Bank" model from the architecture spec.
// It tracks a global budget and per-agent budgets, enforcing the Kill Switch
// when any agent's budget reaches zero.
type BudgetManager struct {
	mu           sync.RWMutex
	globalBudget float64
	agentBudgets map[string]float64 // key = Agent ID
}

// NewBudgetManager creates a new BudgetManager with the given global budget.
func NewBudgetManager(globalBudget float64) *BudgetManager {
	return &BudgetManager{
		globalBudget: globalBudget,
		agentBudgets: make(map[string]float64),
	}
}

// RegisterAgent assigns an initial budget to a new agent.
func (bm *BudgetManager) RegisterAgent(agentID string, budget float64) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if budget > bm.globalBudget {
		return fmt.Errorf("cannot allocate %.4f: global budget only has %.4f remaining", budget, bm.globalBudget)
	}

	bm.agentBudgets[agentID] = budget
	return nil
}

// Deduct subtracts the cost of an API call from the agent's budget.
// Returns true if the agent is now bankrupt (Kill Switch triggered).
func (bm *BudgetManager) Deduct(agentID string, cost float64) (bankrupt bool, err error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	budget, ok := bm.agentBudgets[agentID]
	if !ok {
		return false, fmt.Errorf("agent %s not found in budget manager", agentID)
	}

	budget -= cost
	bm.agentBudgets[agentID] = budget
	bm.globalBudget -= cost

	// KILL SWITCH: if budget <= 0, agent must die immediately (Rule #2)
	if budget <= 0 {
		return true, nil
	}
	return false, nil
}

// GetBudget returns the remaining budget for an agent.
func (bm *BudgetManager) GetBudget(agentID string) (float64, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	budget, ok := bm.agentBudgets[agentID]
	if !ok {
		return 0, fmt.Errorf("agent %s not found in budget manager", agentID)
	}
	return budget, nil
}

// GetGlobalBudget returns the remaining global budget.
func (bm *BudgetManager) GetGlobalBudget() float64 {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.globalBudget
}

// AllocateFromParent transfers a fraction of the parent's budget to a child.
// Returns the allocated amount, or an error if the parent has insufficient funds.
func (bm *BudgetManager) AllocateFromParent(parentID string, fraction float64) (float64, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	parentBudget, ok := bm.agentBudgets[parentID]
	if !ok {
		return 0, fmt.Errorf("parent agent %s not found in budget manager", parentID)
	}

	allocated := parentBudget * fraction
	if allocated <= 0 {
		return 0, fmt.Errorf("parent %s has insufficient budget (%.4f) for allocation", parentID, parentBudget)
	}

	bm.agentBudgets[parentID] -= allocated
	return allocated, nil
}

// RemoveAgent removes an agent from budget tracking.
func (bm *BudgetManager) RemoveAgent(agentID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.agentBudgets, agentID)
}

// IsBankrupt checks if a specific agent is bankrupt without modifying state.
func (bm *BudgetManager) IsBankrupt(agentID string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	budget, ok := bm.agentBudgets[agentID]
	if !ok {
		return true
	}
	return budget <= 0
}
