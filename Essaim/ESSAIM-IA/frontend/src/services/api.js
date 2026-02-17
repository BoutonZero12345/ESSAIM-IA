const API_BASE = 'http://localhost:8080';

/**
 * API service for controlling the ESSAIM system.
 * Per Architecture §10 (Frontend services).
 */
const api = {
    /**
     * Start a new mission by injecting the Alpha agent.
     * @param {string} objective - The mission objective
     * @param {number} budget - Optional budget override
     */
    async start(objective, budget) {
        const res = await fetch(`${API_BASE}/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ objective, budget }),
        });
        if (!res.ok) throw new Error(`Start failed: ${res.statusText}`);
        return res.json();
    },

    /**
     * Health check endpoint.
     */
    async health() {
        const res = await fetch(`${API_BASE}/health`);
        if (!res.ok) throw new Error(`Health check failed: ${res.statusText}`);
        return res.json();
    },
};

export default api;
