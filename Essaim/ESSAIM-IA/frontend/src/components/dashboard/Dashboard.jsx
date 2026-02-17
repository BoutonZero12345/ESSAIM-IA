import useStore from '../../store/useStore';

/**
 * Dashboard - Displays system metrics: active agents, budget, total spawned.
 * Per Architecture §10 (Cockpit).
 */
export default function Dashboard() {
    const { activeAgents, totalSpawned, globalBudget, connected, systemAlerts } = useStore();

    return (
        <div className="flex flex-col gap-3 p-4">
            {/* Connection Status */}
            <div className="flex items-center gap-2 text-xs mb-1">
                <span className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-500 animate-pulse' : 'bg-red-500'}`} />
                <span className="text-gray-400">{connected ? 'CONNECTÉ' : 'DÉCONNECTÉ'}</span>
            </div>

            {/* Metric Cards */}
            <div className="grid grid-cols-3 gap-2">
                <MetricCard label="Agents Actifs" value={activeAgents} color="text-blue-400" />
                <MetricCard label="Total Spawned" value={totalSpawned} color="text-emerald-400" />
                <MetricCard label="Budget Global" value={`${globalBudget.toFixed(0)}`} color="text-amber-400" />
            </div>

            {/* System Alerts */}
            {systemAlerts.length > 0 && (
                <div className="mt-2">
                    <div className="text-xs font-semibold text-red-400 mb-1">ALERTES</div>
                    <div className="flex flex-col gap-1 max-h-24 overflow-y-auto">
                        {systemAlerts.slice(-5).map((alert, i) => (
                            <div
                                key={i}
                                className="text-xs px-2 py-1 bg-red-900/30 border border-red-700/30 rounded text-red-300"
                            >
                                {alert.message || JSON.stringify(alert)}
                            </div>
                        ))}
                    </div>
                </div>
            )}
        </div>
    );
}

function MetricCard({ label, value, color }) {
    return (
        <div className="bg-gray-800/50 backdrop-blur-sm rounded-lg p-3 border border-gray-700/30">
            <div className="text-xs text-gray-500 uppercase tracking-wider">{label}</div>
            <div className={`text-2xl font-bold ${color} mt-1`}>{value}</div>
        </div>
    );
}
