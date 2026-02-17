import { useEffect, useRef, useCallback } from 'react';
import useStore from '../store/useStore';

const WS_URL = 'ws://localhost:8080/ws';
const RECONNECT_DELAY = 3000;

/**
 * useSocket hook - Listens to the backend WebSocket on port 8080.
 * Handles GRAPH_UPDATE, AGENT_STATE, LOG_STREAM, SYSTEM_ALERT events.
 * Per Architecture §10.3.
 */
export default function useSocket() {
    const wsRef = useRef(null);
    const reconnectTimer = useRef(null);

    const {
        addAgent,
        updateAgentState,
        removeAgent,
        addLog,
        setConnected,
        addSystemAlert,
        setGlobalBudget,
        resetState,
    } = useStore();

    const connect = useCallback(() => {
        // Prevent duplicate connections
        if (wsRef.current) {
            const state = wsRef.current.readyState;
            if (state === WebSocket.OPEN || state === WebSocket.CONNECTING) return;
        }

        const ws = new WebSocket(WS_URL);
        wsRef.current = ws;

        ws.onopen = () => {
            // If we had a previous connection, this is a reconnect → reset state
            if (reconnectTimer.current) {
                resetState();
            }
            setConnected(true);
            addLog('WebSocket connecté au backend ✅', 'system');
            if (reconnectTimer.current) {
                clearTimeout(reconnectTimer.current);
                reconnectTimer.current = null;
            }
        };

        ws.onclose = (event) => {
            setConnected(false);
            const reason = event.reason || `code=${event.code}`;
            addLog(`WebSocket déconnecté (${reason}). Backend lancé ? → go run ./cmd/server/main.go`, 'warning');
            addLog(`Reconnexion dans ${RECONNECT_DELAY / 1000}s vers ${WS_URL}...`, 'warning');
            reconnectTimer.current = setTimeout(connect, RECONNECT_DELAY);
        };

        ws.onerror = () => {
            addLog(`❌ Impossible de joindre le backend sur ${WS_URL}`, 'error');
            addLog('💡 Assurez-vous que le serveur Go tourne : go run ./cmd/server/main.go', 'error');
        };

        ws.onmessage = (event) => {
            try {
                const { type, payload } = JSON.parse(event.data);

                switch (type) {
                    case 'GRAPH_UPDATE':
                        handleGraphUpdate(payload);
                        break;
                    case 'AGENT_STATE':
                        handleAgentState(payload);
                        break;
                    case 'LOG_STREAM':
                        handleLogStream(payload);
                        break;
                    case 'SYSTEM_ALERT':
                        handleSystemAlert(payload);
                        break;
                    default:
                        addLog(`Unknown event type: ${type}`, 'warning');
                }
            } catch (err) {
                addLog(`Failed to parse message: ${err.message}`, 'error');
            }
        };
    }, [addAgent, updateAgentState, removeAgent, addLog, setConnected, addSystemAlert, setGlobalBudget, resetState]);

    const handleGraphUpdate = useCallback((payload) => {
        if (payload.action === 'ADD_NODE' && payload.agent) {
            addAgent(payload.agent);
            addLog(`Agent ${payload.agent.id.slice(0, 8)} spawned (${payload.agent.role})`, 'info');
        } else if (payload.action === 'REMOVE_NODE' && payload.agentId) {
            removeAgent(payload.agentId);
            addLog(`Agent ${payload.agentId.slice(0, 8)} removed`, 'warning');
        }
    }, [addAgent, removeAgent, addLog]);

    const handleAgentState = useCallback((payload) => {
        if (payload.agentId && payload.status) {
            updateAgentState(payload.agentId, { status: payload.status });
            addLog(`Agent ${payload.agentId.slice(0, 8)} → ${payload.status}`, 'info');
        }
    }, [updateAgentState, addLog]);

    const handleLogStream = useCallback((payload) => {
        const message = payload.message || JSON.stringify(payload);
        addLog(message, 'info');
    }, [addLog]);

    const handleSystemAlert = useCallback((payload) => {
        addSystemAlert(payload);
        addLog(`⚠ ALERT: ${payload.message || JSON.stringify(payload)}`, 'error');
    }, [addSystemAlert, addLog]);

    useEffect(() => {
        connect();
        return () => {
            if (wsRef.current) wsRef.current.close();
            if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
        };
    }, [connect]);

    return { ws: wsRef.current };
}
