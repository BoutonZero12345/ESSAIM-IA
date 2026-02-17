import { create } from 'zustand';

/**
 * Main Zustand store for ESSAIM IA state management.
 * Tracks agents (graph nodes/links), logs, and system metrics.
 */
const useStore = create((set, get) => ({
    // === Graph Data ===
    agents: {},       // Map<agentId, agentObject>
    links: [],        // Array of { source, target }

    // === Logs ===
    logs: [],         // Array of { timestamp, message, level }

    // === Metrics ===
    globalBudget: 0,
    activeAgents: 0,
    totalSpawned: 0,

    // === Selected Agent (God Mode) ===
    selectedAgent: null,

    // === System Status ===
    connected: false,
    systemAlerts: [],

    // === Actions ===
    addAgent: (agent) => set((state) => {
        const agents = { ...state.agents, [agent.id]: agent };
        const links = [...state.links];

        // Add link from parent to child
        if (agent.parentId) {
            links.push({ source: agent.parentId, target: agent.id });
        }

        return {
            agents,
            links,
            activeAgents: Object.keys(agents).length,
            totalSpawned: state.totalSpawned + 1,
        };
    }),

    updateAgentState: (agentId, updates) => set((state) => {
        const agent = state.agents[agentId];
        if (!agent) return state;

        return {
            agents: {
                ...state.agents,
                [agentId]: { ...agent, ...updates },
            },
        };
    }),

    removeAgent: (agentId) => set((state) => {
        const agents = { ...state.agents };
        delete agents[agentId];

        return {
            agents,
            links: state.links.filter(
                (l) => l.source !== agentId && l.target !== agentId
            ),
            activeAgents: Object.keys(agents).length,
        };
    }),

    addLog: (message, level = 'info') => set((state) => ({
        logs: [
            ...state.logs.slice(-499), // Keep last 500 logs
            { timestamp: Date.now(), message, level },
        ],
    })),

    setSelectedAgent: (agent) => set({ selectedAgent: agent }),

    setConnected: (connected) => set({ connected }),

    addSystemAlert: (alert) => set((state) => ({
        systemAlerts: [
            ...state.systemAlerts.slice(-19),
            { timestamp: Date.now(), ...alert },
        ],
    })),

    setGlobalBudget: (budget) => set({ globalBudget: budget }),

    // Reset all state (called on reconnect / server restart)
    resetState: () => set({
        agents: {},
        links: [],
        logs: [],
        globalBudget: 0,
        activeAgents: 0,
        totalSpawned: 0,
        selectedAgent: null,
        systemAlerts: [],
    }),

    // === Graph Data Getter (for react-force-graph-2d) ===
    getGraphData: () => {
        const state = get();
        const nodes = Object.values(state.agents).map((agent) => ({
            id: agent.id,
            role: agent.role,
            status: agent.status,
            budget: agent.budget,
            parentId: agent.parentId,
        }));
        return { nodes, links: state.links };
    },
}));

export default useStore;
