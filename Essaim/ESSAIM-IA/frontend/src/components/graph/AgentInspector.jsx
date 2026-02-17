import useStore from '../../store/useStore';

const ROLE_COLORS = {
    ARCHITECT: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
    WORKER: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
    CRITIC: 'bg-red-500/20 text-red-400 border-red-500/30',
    CODER: 'bg-violet-500/20 text-violet-400 border-violet-500/30',
};

/**
 * AgentInspector - "God Mode" panel for inspecting a selected agent.
 * Shows system prompt, memory, budget, and provides TERMINATE BRANCH button.
 * Per Architecture §10.2.
 */
export default function AgentInspector() {
    const { selectedAgent, setSelectedAgent } = useStore();

    if (!selectedAgent) {
        return (
            <div className="flex items-center justify-center h-full text-gray-600 text-sm italic p-4">
                Cliquez sur un nœud pour l&apos;inspecter
            </div>
        );
    }

    const roleStyle = ROLE_COLORS[selectedAgent.role] || 'bg-gray-500/20 text-gray-400 border-gray-500/30';

    return (
        <div className="flex flex-col h-full overflow-y-auto">
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-3 border-b border-gray-700/30">
                <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
                        Inspecteur
                    </span>
                    <span className={`text-xs px-2 py-0.5 rounded-full border ${roleStyle}`}>
                        {selectedAgent.role}
                    </span>
                </div>
                <button
                    onClick={() => setSelectedAgent(null)}
                    className="text-gray-500 hover:text-gray-300 text-lg cursor-pointer"
                >
                    ×
                </button>
            </div>

            {/* Agent Info */}
            <div className="p-4 space-y-3">
                <InfoRow label="ID" value={selectedAgent.id} mono />
                <InfoRow label="Parent" value={selectedAgent.parentId || 'ROOT'} mono />
                <InfoRow label="Statut" value={selectedAgent.status} />
                <InfoRow label="Budget" value={`${(selectedAgent.budget || 0).toFixed(2)} tokens`} />

                {/* Memory (last messages) */}
                {selectedAgent.memory && selectedAgent.memory.length > 0 && (
                    <div>
                        <div className="text-xs font-semibold text-gray-500 uppercase mb-1">Mémoire</div>
                        <div className="bg-gray-900 rounded-lg p-2 max-h-40 overflow-y-auto space-y-1">
                            {selectedAgent.memory.slice(-5).map((msg, i) => (
                                <div key={i} className="text-xs">
                                    <span className="text-gray-500">{msg.role}:</span>{' '}
                                    <span className="text-gray-300 break-all">{msg.content?.slice(0, 200)}</span>
                                </div>
                            ))}
                        </div>
                    </div>
                )}

                {/* Kill Button */}
                <button
                    className="w-full mt-4 py-2 px-4 bg-red-900/40 border border-red-700/40 rounded-lg
                     text-red-400 text-xs font-bold uppercase tracking-wider
                     hover:bg-red-800/50 hover:border-red-600/50 transition-all cursor-pointer"
                    onClick={() => {
                        // TODO: Send TERMINATE_BRANCH command to backend
                        alert(`TERMINATE BRANCH: ${selectedAgent.id}`);
                    }}
                >
                    ⚠ TERMINATE BRANCH
                </button>
            </div>
        </div>
    );
}

function InfoRow({ label, value, mono }) {
    return (
        <div className="flex justify-between items-center text-xs">
            <span className="text-gray-500 uppercase">{label}</span>
            <span className={`text-gray-300 ${mono ? 'font-mono' : ''} truncate max-w-[65%] text-right`}>
                {value}
            </span>
        </div>
    );
}
