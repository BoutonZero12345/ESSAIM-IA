import React, { useState, useEffect, useRef } from 'react'
import { Activity, Play, Send, Sparkles, Terminal, FileText, ChevronRight, X, AlertTriangle } from 'lucide-react'
import MetricsPanel from './components/MetricsPanel'
import SwarmGraph from './components/SwarmGraph'
import Timeline from './components/Timeline'

const WEBSOCKET_URL = 'ws://localhost:8080/ws'
const API_START_URL = 'http://localhost:8080/start'

export default function App() {
  // WebSocket and Events Storage
  const [events, setEvents] = useState([])
  const [isLive, setIsLive] = useState(true)
  const [playbackIndex, setPlaybackIndex] = useState(-1)
  const [wsStatus, setWsStatus] = useState('disconnected') // 'connected', 'disconnected', 'connecting'
  const [isPlaying, setIsPlaying] = useState(false)
  
  // App States
  const [objectiveInput, setObjectiveInput] = useState('Rédiger un roman fantastique de 10 chapitres sur un mage cybernétique')
  const [budgetInput, setBudgetInput] = useState(5.0)
  const [selectedAgent, setSelectedAgent] = useState(null)
  
  // References
  const wsRef = useRef(null)
  const playbackIntervalRef = useRef(null)
  const logEndRef = useRef(null)
  const reconnectTimeoutRef = useRef(null)

  // 1. WebSocket connection setup
  const connectWebSocket = () => {
    setWsStatus('connecting')
    const socket = new WebSocket(WEBSOCKET_URL)
    wsRef.current = socket

    socket.onopen = () => {
      if (socket !== wsRef.current) return
      setWsStatus('connected')
      console.log('[WS] Connecté au serveur ESSAIM-IA')
    }

    socket.onmessage = (eventMsg) => {
      if (socket !== wsRef.current) return
      try {
        const rawEvent = JSON.parse(eventMsg.data)
        
        setEvents((prev) => {
          const updated = [...prev, {
            ...rawEvent,
            id: `${rawEvent.type}-${Date.now()}-${Math.random()}`,
            timestamp: Date.now()
          }]
          
          // Auto-advance if live
          return updated
        })
      } catch (err) {
        console.error('[WS] Erreur parsing message:', err)
      }
    }

    socket.onclose = () => {
      if (socket !== wsRef.current) return
      setWsStatus('disconnected')
      console.log('[WS] Déconnecté du serveur Go, tentative de reconnexion dans 3s...')
      
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = setTimeout(connectWebSocket, 3000)
    }

    socket.onerror = (err) => {
      if (socket !== wsRef.current) return
      console.warn('[WS] WebSocket hors-ligne (Le serveur Go n\'est probablement pas démarré)')
    }
  }

  useEffect(() => {
    connectWebSocket()
    return () => {
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
    }
  }, [])

  // Auto-advance playbackIndex when new events arrive in LIVE mode
  useEffect(() => {
    if (isLive && events.length > 0) {
      setPlaybackIndex(events.length - 1)
    }
  }, [events, isLive])

  // Playback player animation effect
  useEffect(() => {
    if (isPlaying) {
      setIsLive(false) // Exit live mode when playing old sequence
      playbackIntervalRef.current = setInterval(() => {
        setPlaybackIndex((prev) => {
          if (prev >= events.length - 1) {
            setIsPlaying(false)
            return prev
          }
          return prev + 1
        })
      }, 800) // speed step
    } else {
      if (playbackIntervalRef.current) clearInterval(playbackIntervalRef.current)
    }

    return () => {
      if (playbackIntervalRef.current) clearInterval(playbackIntervalRef.current)
    }
  }, [isPlaying, events.length])

  // Replay timeline index change handler
  const handleIndexChange = (index) => {
    setIsPlaying(false)
    setIsLive(index === events.length - 1)
    setPlaybackIndex(index)
  }

  // Live jump sync button handler
  const handleGoLive = () => {
    setIsPlaying(false)
    setIsLive(true)
    if (events.length > 0) {
      setPlaybackIndex(events.length - 1)
    }
  }

  // Start Genesis mission trigger
  const handleStartMission = async (e) => {
    e.preventDefault()
    if (!objectiveInput.trim()) return

    setEvents([]) // Reset session
    setPlaybackIndex(-1)
    setIsLive(true)

    try {
      const response = await fetch(API_START_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          objective: objectiveInput,
          budget: budgetInput
        })
      })

      if (!response.ok) {
        const errData = await response.json()
        alert(`Échec de lancement: ${errData.error || 'Erreur inconnue'}`)
      }
    } catch (err) {
      console.error('[API] Erreur lancement:', err)
      alert(`Erreur connexion au serveur: ${err.message}`)
    }
  }

  // 2. TIMELINE STATE RECONSTRUCTION ALGORITHM
  const reconstructedState = useMemoState(events, playbackIndex)

  // Auto-scroll logs
  useEffect(() => {
    if (logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [reconstructedState.logs.length])

  // Get active agent if selected
  const activeSelectedAgent = reconstructedState.nodes.find(n => n.ID === selectedAgent?.ID) || selectedAgent

  return (
    <div className="app-container">
      {/* 1. HEADER COCKPIT */}
      <header className="header">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🐝</span>
          <div className="flex flex-col">
            <h1 className="text-base font-bold tracking-wider uppercase text-white flex items-center gap-2">
              ESSAIM-IA <span className="text-[10px] font-semibold text-[var(--text-muted)] tracking-widest px-1.5 py-0.5 rounded border border-[var(--border-color)]">V1.0</span>
            </h1>
            <span className="text-[10px] text-[var(--text-muted)] font-mono font-medium tracking-tight">
              {wsStatus === 'connected' ? (
                <span className="text-green-400 flex items-center gap-1">
                  <span className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse"></span> Connecté (ws://localhost:8080)
                </span>
              ) : wsStatus === 'connecting' ? (
                <span className="text-yellow-400 flex items-center gap-1">
                  <span className="w-1.5 h-1.5 rounded-full bg-yellow-400 animate-ping"></span> Connexion en cours...
                </span>
              ) : (
                <span className="text-red-400 flex items-center gap-1">
                  <span className="w-1.5 h-1.5 rounded-full bg-red-400"></span> Déconnecté du serveur
                </span>
              )}
            </span>
          </div>
        </div>

        {/* Coût & jetons en direct */}
        <MetricsPanel 
          totalTokens={reconstructedState.totalTokens}
          totalCost={reconstructedState.totalCost}
          agentCount={reconstructedState.nodes.length}
          activeAgentCount={reconstructedState.nodes.filter(n => n.Status === 'working' || n.Status === 'born').length}
          isLive={isLive}
        />
      </header>

      {/* 2. BODY CONTENT */}
      <div className="main-content">
        
        {/* Left/Center Graph Viewport */}
        <div className="flex-1 flex flex-col relative h-full">
          {reconstructedState.nodes.length === 0 ? (
            /* GENESIS FORM */
            <div className="flex-1 flex items-center justify-center p-6">
              <div className="glass-panel p-8 w-full max-w-xl flex flex-col gap-6" style={{ background: 'rgba(13, 17, 27, 0.8)' }}>
                <div className="flex items-center gap-3 border-b border-[var(--border-color)] pb-4">
                  <Sparkles size={24} className="text-blue-400 animate-pulse" />
                  <div>
                    <h2 className="text-lg font-bold text-white tracking-wide uppercase">Lancer une nouvelle Mission Alpha</h2>
                    <p className="text-xs text-[var(--text-muted)]">Configurez l'objectif global et l'enveloppe budgétaire de l'essaim.</p>
                  </div>
                </div>

                <form onSubmit={handleStartMission} className="flex flex-col gap-4">
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider">Objectif de la Mission</label>
                    <textarea
                      value={objectiveInput}
                      onChange={(e) => setObjectiveInput(e.target.value)}
                      placeholder="Rédigez l'instruction macro..."
                      className="w-full h-24 bg-[var(--bg-primary)] border border-[var(--border-color)] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 font-sans resize-none transition-all"
                    />
                  </div>

                  <div className="flex gap-4">
                    <div className="flex-1 flex flex-col gap-1.5">
                      <label className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider">Budget Initial Max (€)</label>
                      <input
                        type="number"
                        step="0.5"
                        min="1"
                        value={isNaN(budgetInput) || budgetInput === '' ? '' : budgetInput}
                        onChange={(e) => {
                          const parsed = parseFloat(e.target.value)
                          setBudgetInput(isNaN(parsed) ? '' : parsed)
                        }}
                        className="bg-[var(--bg-primary)] border border-[var(--border-color)] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 font-mono transition-all"
                      />
                    </div>
                    <div className="flex-1 flex flex-col gap-1.5 justify-end">
                      <span className="text-[10px] text-[var(--text-muted)] font-mono leading-relaxed">
                        Équivaut à environ :<br />
                        <strong className="text-[var(--text-secondary)] font-bold">{((budgetInput || 0) * 1000000).toLocaleString()} tokens</strong> Gemini.
                      </span>
                    </div>
                  </div>

                  {wsStatus !== 'connected' && (
                    <div className="glass-panel p-3 flex gap-3 items-center" style={{ borderColor: 'rgba(255, 23, 68, 0.25)', background: 'rgba(255, 23, 68, 0.05)' }}>
                      <AlertTriangle size={18} className="text-red-400 animate-pulse" />
                      <div className="flex flex-col">
                        <span className="text-xs font-bold text-red-400 uppercase">Serveur Backend Hors-ligne</span>
                        <span className="text-[10px] text-[var(--text-muted)] leading-tight">
                          Veuillez démarrer le serveur Go (<code>go run cmd/server/main.go</code>) pour pouvoir injecter l'agent Alpha.
                        </span>
                      </div>
                    </div>
                  )}

                  <button
                    type="submit"
                    disabled={wsStatus !== 'connected'}
                    className="mt-2 bg-blue-500 hover:bg-blue-600 text-white font-bold py-3 px-6 rounded-lg text-sm uppercase tracking-wider flex items-center justify-center gap-2 transition-all transform active:scale-95 disabled:opacity-40 disabled:hover:bg-blue-500 disabled:active:scale-100 disabled:cursor-not-allowed"
                    style={{
                      boxShadow: wsStatus === 'connected' ? '0 0 15px rgba(59,130,246,0.3)' : 'none'
                    }}
                  >
                    <Send size={14} />
                    <span>Injecter l'Agent Alpha</span>
                  </button>
                </form>
              </div>
            </div>
          ) : (
            /* ACTIVE GRAPH SYSTEM */
            <div className="flex-1 relative flex items-center justify-center">
              <SwarmGraph 
                nodes={reconstructedState.nodes}
                activeAgentId={activeSelectedAgent?.ID}
                onSelectAgent={setSelectedAgent}
              />
              
              {/* Floating current objective bar */}
              <div className="absolute top-6 left-6 max-w-lg glass-panel px-4 py-2 flex items-center gap-3">
                <FileText size={14} className="text-blue-400" />
                <div className="flex flex-col">
                  <span className="text-[9px] uppercase tracking-wider text-[var(--text-muted)] font-bold">Objectif Mission</span>
                  <span className="text-xs text-[var(--text-secondary)] font-medium truncate max-w-sm" title={reconstructedState.objective}>
                    {reconstructedState.objective || 'Mission Alpha'}
                  </span>
                </div>
              </div>
            </div>
          )}

          {/* Chronological terminal-like logging stream at bottom of graph */}
          {reconstructedState.nodes.length > 0 && (
            <div className="h-44 border-t border-[var(--border-color)] bg-[rgba(7,9,14,0.65)] backdrop-filter var(--glass-blur) flex flex-col">
              <div className="flex justify-between items-center px-4 py-2 border-b border-[var(--border-color)]">
                <div className="flex items-center gap-2 text-[var(--text-secondary)] font-semibold text-xs tracking-wider uppercase">
                  <Terminal size={12} className="text-[var(--role-worker)]" />
                  <span>Flux Tactique / Journaux de Communication</span>
                </div>
                <span className="text-[9px] font-mono text-[var(--text-muted)]">
                  Affichage des logs jusqu'à l'index {playbackIndex + 1}
                </span>
              </div>
              <div className="flex-1 overflow-y-auto p-4 font-mono text-xs flex flex-col gap-1.5">
                {reconstructedState.logs.length === 0 ? (
                  <span className="text-[var(--text-muted)] italic">Aucun log enregistré...</span>
                ) : (
                  reconstructedState.logs.map((logItem, idx) => (
                    <div key={idx} className="flex gap-2">
                      <span className="text-[var(--text-muted)]">[{new Date(logItem.timestamp).toLocaleTimeString()}]</span>
                      <span className={logItem.message.includes('💀') ? 'text-red-400' : logItem.message.includes('✅') ? 'text-green-400' : logItem.message.includes('🏤') ? 'text-purple-400' : 'text-slate-300'}>
                        {logItem.message}
                      </span>
                    </div>
                  ))
                )}
                <div ref={logEndRef} />
              </div>
            </div>
          )}
        </div>

        {/* 3. RIGHT PANEL - AGENT INSPECTOR */}
        {reconstructedState.nodes.length > 0 && (
          <div className={`inspector-panel ${activeSelectedAgent ? '' : 'translate-x-full'}`} style={{ width: activeSelectedAgent ? '380px' : '0px', minWidth: activeSelectedAgent ? '380px' : '0px', overflow: 'hidden', borderLeft: activeSelectedAgent ? '1px solid var(--border-color)' : 'none' }}>
            {activeSelectedAgent && (
              <div className="p-6 flex flex-col h-full gap-5 overflow-y-auto">
                {/* Header Inspector */}
                <div className="flex justify-between items-start border-b border-[var(--border-color)] pb-4">
                  <div className="flex flex-col gap-1">
                    <span className="text-[10px] font-mono font-bold tracking-widest text-[var(--text-muted)] uppercase">Fiche d'Agent</span>
                    <h3 className="text-base font-bold text-white uppercase tracking-wider flex items-center gap-2">
                      <span className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: getRoleColor(activeSelectedAgent.Role), boxShadow: `0 0 8px ${getRoleColor(activeSelectedAgent.Role)}` }}></span>
                      {activeSelectedAgent.Role}
                    </h3>
                    <span className="text-[9px] font-mono text-[var(--text-muted)]">{activeSelectedAgent.ID}</span>
                  </div>
                  <button 
                    onClick={() => setSelectedAgent(null)}
                    className="p-1 rounded-md border border-[var(--border-color)] text-[var(--text-muted)] hover:text-white hover:bg-[var(--bg-tertiary)]"
                  >
                    <X size={14} />
                  </button>
                </div>

                {/* Metrics detail widget */}
                <div className="grid grid-cols-2 gap-4">
                  <div className="glass-panel p-3 flex flex-col" style={{ background: 'rgba(7, 9, 14, 0.4)' }}>
                    <span className="text-[9px] text-[var(--text-muted)] uppercase font-semibold">Statut</span>
                    <span className={`text-xs font-bold uppercase mt-1 ${activeSelectedAgent.Status === 'dead' ? 'text-red-400' : activeSelectedAgent.Status === 'working' ? 'text-green-400 animate-pulse' : 'text-blue-400'}`}>
                      {activeSelectedAgent.Status || 'INCONNU'}
                    </span>
                  </div>
                  <div className="glass-panel p-3 flex flex-col" style={{ background: 'rgba(7, 9, 14, 0.4)' }}>
                    <span className="text-[9px] text-[var(--text-muted)] uppercase font-semibold">Enveloppe Jetons</span>
                    <span className="text-xs font-bold text-white mt-1 font-mono">
                      {(activeSelectedAgent.Budget || 0).toLocaleString(undefined, { maximumFractionDigits: 0 })} <span className="text-[9px] text-[var(--text-muted)] font-normal">tks</span>
                    </span>
                  </div>
                </div>

                {/* Bubble info */}
                {activeSelectedAgent.BubbleID && (
                  <div className="glass-panel px-4 py-2 text-xs flex justify-between items-center" style={{ background: 'rgba(189, 0, 255, 0.03)', borderColor: 'rgba(189, 0, 255, 0.15)' }}>
                    <span className="text-[var(--text-muted)]">Bulle de rattachement</span>
                    <span className="font-mono text-purple-400 text-[10px] font-semibold">{activeSelectedAgent.BubbleID.substring(0, 8)}...</span>
                  </div>
                )}

                {/* Agent Memory / System prompt details */}
                <div className="flex flex-col gap-2 flex-1 min-h-[200px]">
                  <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] tracking-wider">Objectif de la Bulle / Instruction</span>
                  <div className="glass-panel p-3 text-xs leading-relaxed text-[var(--text-secondary)] font-mono overflow-y-auto flex-1 bg-[rgba(7,9,14,0.3)]">
                    {activeSelectedAgent.TaskDescription || (
                      <span className="text-[var(--text-muted)] italic">Aucune instruction directe stockée (Agent de planification).</span>
                    )}
                  </div>
                </div>

                {/* Warn if bankrupt */}
                {activeSelectedAgent.Budget <= 0 && (
                  <div className="glass-panel p-3 flex gap-3 items-center" style={{ borderColor: 'rgba(255, 23, 68, 0.2)', background: 'rgba(255, 23, 68, 0.05)' }}>
                    <AlertTriangle size={18} className="text-red-400 animate-bounce" />
                    <div className="flex flex-col">
                      <span className="text-xs font-bold text-red-400 uppercase">Banqueroute déclenchée</span>
                      <span className="text-[9px] text-[var(--text-muted)] leading-tight">Cet agent a épuisé son budget. Kill Switch activé.</span>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>

      {/* 4. BOTTOM TIMELINE CONTROLS */}
      {events.length > 0 && (
        <Timeline
          eventsCount={events.length}
          currentIndex={playbackIndex}
          onIndexChange={handleIndexChange}
          isLive={isLive}
          onGoLive={handleGoLive}
          isPlaying={isPlaying}
          onTogglePlay={() => setIsPlaying(!isPlaying)}
        />
      )}
    </div>
  )
}

// ==========================================================================
// STATE RECONSTRUCTION HOOK/HELPER (THE TIME-TRAVEL ENGINE)
// ==========================================================================
function useMemoState(events, playbackIndex) {
  return React.useMemo(() => {
    const nodes = []
    const logs = []
    let totalTokens = 0
    let objective = ''

    // Loop through events chronologically up to the current index
    for (let i = 0; i <= playbackIndex; i++) {
      const ev = events[i]
      if (!ev) continue

      if (ev.type === 'GRAPH_UPDATE') {
        const payload = ev.payload || {}
        
        if (payload.action === 'ADD_NODE' && payload.agent) {
          const rawAgent = payload.agent
          const agentId = rawAgent.id || rawAgent.ID

          // Remove if duplicate to avoid issues
          const existingIdx = nodes.findIndex(n => (n.id || n.ID) === agentId)
          if (existingIdx !== -1) nodes.splice(existingIdx, 1)

          // Normalize the agent object to have both Go camelCase/lowercase tags and JSX uppercase tags
          const normalized = {
            ...rawAgent,
            id: agentId,
            ID: agentId,
            bubbleId: rawAgent.bubbleId || rawAgent.BubbleID,
            BubbleID: rawAgent.bubbleId || rawAgent.BubbleID,
            postierId: rawAgent.postierId || rawAgent.PostierID,
            PostierID: rawAgent.postierId || rawAgent.PostierID,
            role: rawAgent.role || rawAgent.Role,
            Role: rawAgent.role || rawAgent.Role,
            status: (rawAgent.status || rawAgent.Status || 'born').toLowerCase(),
            Status: (rawAgent.status || rawAgent.Status || 'born').toLowerCase(),
            budget: rawAgent.budget !== undefined ? rawAgent.budget : rawAgent.Budget,
            Budget: rawAgent.budget !== undefined ? rawAgent.budget : rawAgent.Budget,
            TaskDescription: rawAgent.TaskDescription || rawAgent.taskDescription || ''
          }

          nodes.push(normalized)
        } 
        else if (payload.action === 'REMOVE_NODE' && (payload.agentId || payload.agentID)) {
          const agentId = payload.agentId || payload.agentID
          const idx = nodes.findIndex(n => (n.id || n.ID) === agentId)
          if (idx !== -1) {
            nodes.splice(idx, 1)
          }
        }
      } 
      
      else if (ev.type === 'AGENT_STATE') {
        const payload = ev.payload || {}
        const agentId = payload.agentId || payload.agentID
        const node = nodes.find(n => (n.id || n.ID) === agentId)
        if (node) {
          const status = payload.status || payload.Status
          const budget = payload.budget !== undefined ? payload.budget : payload.Budget
          const role = payload.role || payload.Role

          if (status) {
            node.status = status.toLowerCase()
            node.Status = status.toLowerCase()
          }
          if (budget !== undefined) {
            node.budget = budget
            node.Budget = budget
          }
          if (role) {
            node.role = role
            node.Role = role
          }
        }
      } 
      
      else if (ev.type === 'LOG_STREAM') {
        const payload = ev.payload || {}
        if (payload.message) {
          logs.push({
            message: payload.message,
            timestamp: ev.timestamp
          })
        }
      } 
      
      else if (ev.type === 'SYSTEM_ALERT') {
        const payload = ev.payload || {}
        if (payload.message) {
          logs.push({
            message: `⚠ ALES : ${payload.message}`,
            timestamp: ev.timestamp
          })
        }
        if (payload.result?.objective) {
          objective = payload.result.objective
        }
      }
    }

    // Estimate cumulative budget spent
    const initialBudget = 5000000.0
    const currentBudget = nodes.reduce((sum, n) => sum + (n.budget || n.Budget || 0), 0)
    
    totalTokens = Math.max(0, initialBudget - currentBudget)
    const totalCost = totalTokens / 1000000.0 // 1M tokens = 1 Euro

    return {
      nodes,
      logs,
      totalTokens,
      totalCost,
      objective
    }
  }, [events, playbackIndex])
}

const getRoleColor = (role) => {
  switch (role?.toUpperCase()) {
    case 'ARCHITECT': return 'var(--role-architect)'
    case 'POSTIER': return 'var(--role-postier)'
    case 'RESUMEUR': return 'var(--role-resumeur)'
    case 'CRITIQUE': return 'var(--role-critique)'
    default: return 'var(--role-worker)'
  }
}
