import React from 'react'
import { Play, Pause, SkipBack, SkipForward, Radio } from 'lucide-react'

export default function Timeline({ 
  eventsCount, 
  currentIndex, 
  onIndexChange, 
  isLive, 
  onGoLive, 
  isPlaying, 
  onTogglePlay 
}) {
  const hasEvents = eventsCount > 0
  const isPast = !isLive && currentIndex < eventsCount - 1

  return (
    <div className="glass-panel p-4 flex flex-col gap-3" style={{ background: 'rgba(7, 9, 14, 0.9)', borderBottomLeftRadius: 0, borderBottomRightRadius: 0, borderTopLeftRadius: 16, borderTopRightRadius: 16 }}>
      <div className="flex justify-between items-center">
        {/* Playback Controls */}
        <div className="flex items-center gap-2">
          <button 
            disabled={!hasEvents || currentIndex === 0}
            onClick={() => onIndexChange(Math.max(0, currentIndex - 1))}
            className="p-1.5 rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:text-white hover:bg-[var(--bg-tertiary)] disabled:opacity-40 disabled:hover:bg-transparent"
            title="Étape précédente"
          >
            <SkipBack size={16} />
          </button>
          
          <button 
            disabled={!hasEvents}
            onClick={onTogglePlay}
            className="p-1.5 rounded-lg border border-[var(--border-color)] text-white bg-[var(--bg-tertiary)] hover:bg-slate-800 disabled:opacity-40"
            title={isPlaying ? 'Pause' : 'Play'}
          >
            {isPlaying ? <Pause size={16} /> : <Play size={16} />}
          </button>

          <button 
            disabled={!hasEvents || currentIndex === eventsCount - 1}
            onClick={() => onIndexChange(Math.min(eventsCount - 1, currentIndex + 1))}
            className="p-1.5 rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:text-white hover:bg-[var(--bg-tertiary)] disabled:opacity-40 disabled:hover:bg-transparent"
            title="Étape suivante"
          >
            <SkipForward size={16} />
          </button>
        </div>

        {/* Timeline Slider */}
        <div className="flex-1 mx-6 flex items-center gap-3">
          <span className="text-xs font-mono text-[var(--text-muted)]">0</span>
          <input 
            type="range"
            min={0}
            max={Math.max(0, eventsCount - 1)}
            value={hasEvents ? currentIndex : 0}
            disabled={!hasEvents}
            onChange={(e) => onIndexChange(parseInt(e.target.value))}
            className="w-full h-1.5 rounded-lg appearance-none cursor-pointer bg-slate-800 accent-blue-400"
            style={{
              background: `linear-gradient(to right, var(--role-architect) 0%, var(--role-architect) ${hasEvents ? (currentIndex / (eventsCount - 1)) * 100 : 0}%, #1e293b ${hasEvents ? (currentIndex / (eventsCount - 1)) * 100 : 0}%, #1e293b 100%)`
            }}
          />
          <span className="text-xs font-mono text-[var(--text-secondary)]">
            {hasEvents ? currentIndex + 1 : 0} <span className="text-[var(--text-muted)]">/ {eventsCount}</span>
          </span>
        </div>

        {/* Live / Go Live Badge */}
        <div>
          {isLive ? (
            <div className="live-badge">
              <span className="live-badge-dot"></span>
              <span className="text-xs font-bold tracking-wider uppercase">DIRECT 🔴</span>
            </div>
          ) : (
            <button 
              onClick={onGoLive}
              className="flex items-center gap-1.5 bg-blue-500 hover:bg-blue-600 text-white text-xs font-bold py-1 px-3 rounded-full shadow-lg transition-all transform hover:scale-105 active:scale-95"
            >
              <Radio size={12} className="animate-pulse" />
              <span>REPRENDRE LE DIRECT</span>
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
