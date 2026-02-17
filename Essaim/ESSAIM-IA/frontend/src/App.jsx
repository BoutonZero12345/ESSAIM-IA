import { useState } from 'react';
import useSocket from './hooks/useSocket';
import NetworkGraph from './components/graph/NetworkGraph';
import AgentInspector from './components/graph/AgentInspector';
import Dashboard from './components/dashboard/Dashboard';
import Console from './components/console/Console';
import api from './services/api';

/**
 * ESSAIM IA - Cockpit Application
 * Main layout: Graph (center), Dashboard (top-right), Inspector (right), Console (bottom).
 */
export default function App() {
  useSocket(); // Connect WebSocket on mount

  const [objective, setObjective] = useState('');
  const [starting, setStarting] = useState(false);

  const handleStart = async () => {
    if (!objective.trim()) return;
    setStarting(true);
    try {
      await api.start(objective.trim());
      setObjective('');
    } catch (err) {
      if (err.message.includes('Failed to fetch') || err.message.includes('fetch')) {
        alert('❌ Impossible de contacter le backend.\n\n💡 Le serveur Go doit tourner :\n   cd backend\n   go run ./cmd/server/main.go');
      } else {
        alert(`❌ Erreur: ${err.message}`);
      }
    } finally {
      setStarting(false);
    }
  };

  return (
    <div className="h-screen w-screen flex flex-col bg-[#0a0e17] overflow-hidden">
      {/* Top Bar */}
      <header className="flex items-center justify-between px-4 py-2 border-b border-gray-800/60 bg-gray-900/50 backdrop-blur-sm shrink-0">
        <div className="flex items-center gap-3">
          <div className="text-lg font-bold tracking-tight">
            <span className="text-emerald-400">ESSAIM</span>
            <span className="text-gray-500 font-light ml-1">IA</span>
          </div>
          <span className="text-[10px] text-gray-600 uppercase tracking-widest">v1.0</span>
        </div>

        {/* Mission Input */}
        <div className="flex items-center gap-2 flex-1 max-w-xl mx-8">
          <input
            type="text"
            value={objective}
            onChange={(e) => setObjective(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleStart()}
            placeholder="Entrez un objectif pour l'essaim..."
            className="flex-1 bg-gray-800/60 border border-gray-700/40 rounded-lg px-3 py-1.5 text-sm
                       text-gray-200 placeholder-gray-600 outline-none focus:border-emerald-500/50
                       focus:ring-1 focus:ring-emerald-500/20 transition-all"
          />
          <button
            onClick={handleStart}
            disabled={starting || !objective.trim()}
            className="px-4 py-1.5 bg-emerald-600/80 hover:bg-emerald-500/80 disabled:bg-gray-700
                       disabled:text-gray-500 text-white text-sm font-medium rounded-lg
                       transition-all cursor-pointer disabled:cursor-not-allowed"
          >
            {starting ? '...' : 'LANCER'}
          </button>
        </div>

        <div className="text-xs text-gray-600">HIVE COCKPIT</div>
      </header>

      {/* Main Content */}
      <div className="flex flex-1 overflow-hidden">
        {/* Graph Area (Center) */}
        <div className="flex-1 relative">
          <NetworkGraph />
        </div>

        {/* Right Sidebar */}
        <div className="w-80 border-l border-gray-800/60 flex flex-col bg-gray-900/30 shrink-0">
          {/* Dashboard */}
          <div className="border-b border-gray-800/60">
            <Dashboard />
          </div>

          {/* God Mode Inspector */}
          <div className="flex-1 overflow-hidden">
            <AgentInspector />
          </div>
        </div>
      </div>

      {/* Bottom Console */}
      <div className="h-48 border-t border-gray-800/60 bg-gray-900/30 shrink-0">
        <Console />
      </div>
    </div>
  );
}
