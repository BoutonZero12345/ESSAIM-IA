import React from 'react'
import { Users, Coins, DollarSign } from 'lucide-react'

export default function MetricsPanel({ totalTokens, totalCost, agentCount, activeAgentCount, isLive }) {
  return (
    <div className="flex gap-4 items-center">
      {/* Agents Count */}
      <div className="glass-panel px-4 py-2 flex items-center gap-3" style={{ background: 'rgba(13, 17, 27, 0.4)' }}>
        <Users size={16} className="text-blue-400" style={{ color: 'var(--role-architect)' }} />
        <div className="flex flex-col">
          <span className="text-[10px] uppercase text-[var(--text-muted)] font-semibold tracking-wider">Agents</span>
          <span className="text-sm font-bold tracking-tight">
            {activeAgentCount} <span className="text-[var(--text-muted)] font-normal">/ {agentCount}</span>
          </span>
        </div>
      </div>

      {/* Token Counter */}
      <div className="glass-panel px-4 py-2 flex items-center gap-3" style={{ background: 'rgba(13, 17, 27, 0.4)' }}>
        <Coins size={16} className="text-yellow-500" style={{ color: 'var(--role-resumeur)' }} />
        <div className="flex flex-col">
          <span className="text-[10px] uppercase text-[var(--text-muted)] font-semibold tracking-wider">Consommation</span>
          <span className="text-sm font-bold tracking-tight">
            {totalTokens.toLocaleString()} <span className="text-[10px] font-normal text-[var(--text-muted)]">tokens</span>
          </span>
        </div>
      </div>

      {/* Cost Counter */}
      <div className="glass-panel px-4 py-2 flex items-center gap-3" style={{ 
        background: 'rgba(13, 17, 27, 0.4)',
        borderColor: isLive ? 'rgba(0, 230, 118, 0.15)' : 'var(--border-color)'
      }}>
        <DollarSign size={16} className="text-green-400" style={{ color: 'var(--role-worker)' }} />
        <div className="flex flex-col">
          <span className="text-[10px] uppercase text-[var(--text-muted)] font-semibold tracking-wider">Coût Gemini 2.5</span>
          <span className="text-sm font-bold tracking-tight text-green-400" style={{ color: 'var(--role-worker)' }}>
            {totalCost.toFixed(5)} $
          </span>
        </div>
      </div>
    </div>
  )
}
