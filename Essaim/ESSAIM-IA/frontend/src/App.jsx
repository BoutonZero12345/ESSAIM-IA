import React, { useState, useEffect, useRef } from 'react'
import { Activity, Play, Send, Sparkles, Terminal, FileText, ChevronRight, X, AlertTriangle } from 'lucide-react'
import MetricsPanel from './components/MetricsPanel'
import SwarmGraph from './components/SwarmGraph'
import Timeline from './components/Timeline'

const WEBSOCKET_URL = 'ws://localhost:8080/ws'
const API_START_URL = 'http://localhost:8080/start'
const API_STOP_URL = 'http://localhost:8080/stop'

// ==========================================================================
// TARIFICATION GEMINI 2.5 FLASH LITE (CONSTANTES UNIFIÉES)
// ==========================================================================
// 80% Input à 0.10$/Million, 20% Output à 0.40$/Million -> Tarif blendé = 0.16$ / Million.
const USD_PER_TOKEN = 0.16 / 1000000.0
const TOKENS_PER_DOLLAR = 1.0 / USD_PER_TOKEN // 6250000.0

export default function App() {
  // WebSocket and Events Storage
  const [events, setEvents] = useState(() => {
    try {
      const saved = localStorage.getItem('essaim_events')
      return saved ? JSON.parse(saved) : []
    } catch (e) {
      console.warn('[LOCAL STORAGE] Impossible de charger les événements:', e)
      return []
    }
  })
  const [isLive, setIsLive] = useState(() => {
    const saved = localStorage.getItem('essaim_is_live')
    return saved !== null ? saved === 'true' : true
  })
  const [playbackIndex, setPlaybackIndex] = useState(() => {
    const saved = localStorage.getItem('essaim_playback_index')
    return saved ? parseInt(saved, 10) : -1
  })
  const [wsStatus, setWsStatus] = useState('disconnected') // 'connected', 'disconnected', 'connecting'
  const [isPlaying, setIsPlaying] = useState(false)
  
  // App States
  const [objectiveInput, setObjectiveInput] = useState(() => {
    return localStorage.getItem('essaim_objective') || 'Rédiger un roman fantastique de 10 chapitres sur un mage cybernétique'
  })
  const [budgetInput, setBudgetInput] = useState(() => {
    const saved = localStorage.getItem('essaim_budget')
    return saved ? parseFloat(saved) : 1.0
  })
  const [selectedAgent, setSelectedAgent] = useState(null)
  const [apiTestStatus, setApiTestStatus] = useState('idle') // 'idle', 'testing', 'success', 'error'
  const [apiTestResult, setApiTestResult] = useState(null)
  const [showDiagnostics, setShowDiagnostics] = useState(false)
  const [showStats, setShowStats] = useState(false)
  const [displayUnit, setDisplayUnit] = useState(() => {
    return localStorage.getItem('essaim_display_unit') || 'token' // 'token' or 'usd'
  })
  const [logPanelHeight, setLogPanelHeight] = useState(176) // Default 176px (h-44)

  // Drag resizer handle for logs panel
  const handleMouseDownResize = (e) => {
    e.preventDefault()
    const startY = e.clientY
    const startHeight = logPanelHeight

    const handleMouseMoveResize = (moveEvent) => {
      const deltaY = moveEvent.clientY - startY
      // Clamp between 80px and 500px so it fits well on all screens
      const newHeight = Math.max(80, Math.min(500, startHeight - deltaY))
      setLogPanelHeight(newHeight)
    }

    const handleMouseUpResize = () => {
      window.removeEventListener('mousemove', handleMouseMoveResize)
      window.removeEventListener('mouseup', handleMouseUpResize)
    }

    window.addEventListener('mousemove', handleMouseMoveResize)
    window.addEventListener('mouseup', handleMouseUpResize)
  }

  // Persist states to LocalStorage
  useEffect(() => {
    localStorage.setItem('essaim_display_unit', displayUnit)
  }, [displayUnit])
  useEffect(() => {
    try {
      localStorage.setItem('essaim_events', JSON.stringify(events))
    } catch (e) {
      console.warn('[LOCAL STORAGE] Erreur écriture events:', e)
    }
  }, [events])

  useEffect(() => {
    localStorage.setItem('essaim_is_live', isLive)
  }, [isLive])

  useEffect(() => {
    localStorage.setItem('essaim_playback_index', playbackIndex)
  }, [playbackIndex])

  useEffect(() => {
    localStorage.setItem('essaim_objective', objectiveInput)
  }, [objectiveInput])

  useEffect(() => {
    if (budgetInput !== '' && !isNaN(budgetInput)) {
      localStorage.setItem('essaim_budget', budgetInput)
    }
  }, [budgetInput])
  
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

  // Auto-run API key validation test once backend WebSocket connects
  useEffect(() => {
    if (wsStatus === 'connected') {
      handleTestAPI()
    } else {
      setApiTestStatus('idle')
      setApiTestResult(null)
    }
  }, [wsStatus])

  // Test the Gemini API Key ping in the Go backend
  const handleTestAPI = async () => {
    setApiTestStatus('testing')
    setApiTestResult(null)
    try {
      const response = await fetch('http://localhost:8080/api/test-llm')
      const data = await response.json()
      if (response.ok && data.status === 'ok') {
        setApiTestStatus('success')
        setApiTestResult(data)
      } else {
        setApiTestStatus('error')
        setApiTestResult(data.message || 'Clé API non configurée ou rejetée par le serveur.')
      }
    } catch (err) {
      setApiTestStatus('error')
      setApiTestResult('Impossible de contacter le service de diagnostic du serveur backend.')
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
          budget: budgetInput * (TOKENS_PER_DOLLAR / 1000000.0) // Scale to get exactly TOKENS_PER_DOLLAR tokens after backend's *1M logic
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

  // Reset/Restart Session Handler
  const handleResetSession = () => {
    if (window.confirm('Voulez-vous vraiment réinitialiser la session actuelle et lancer une nouvelle mission ?')) {
      setEvents([])
      setPlaybackIndex(-1)
      setIsLive(true)
      setSelectedAgent(null)
      try {
        localStorage.removeItem('essaim_events')
        localStorage.removeItem('essaim_playback_index')
        localStorage.removeItem('essaim_is_live')
      } catch (e) {
        console.warn('[LOCAL STORAGE] Erreur nettoyage:', e)
      }
    }
  }

  // Stop the entire active swarm network
  const handleStopSwarm = async () => {
    if (window.confirm("Voulez-vous vraiment arrêter l'essaim entier et interrompre la mission active ?")) {
      try {
        const response = await fetch(API_STOP_URL, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' }
        })
        if (response.ok) {
          // Immediately reset session to return to the Genesis creation screen
          setEvents([])
          setPlaybackIndex(-1)
          setIsLive(true)
          setSelectedAgent(null)
          try {
            localStorage.removeItem('essaim_events')
            localStorage.removeItem('essaim_playback_index')
            localStorage.removeItem('essaim_is_live')
          } catch (e) {
            console.warn('[LOCAL STORAGE] Erreur nettoyage:', e)
          }
        } else {
          const errData = await response.json()
          alert(`Échec de l'arrêt de l'essaim: ${errData.error || 'Erreur inconnue'}`)
        }
      } catch (err) {
        console.error('[API] Erreur arrêt:', err)
        alert(`Erreur de connexion au serveur pour arrêter l'essaim: ${err.message}`)
      }
    }
  }

  // 2. TIMELINE STATE RECONSTRUCTION ALGORITHM
  const defaultInitialBudgetInTokens = (budgetInput || 1.0) * TOKENS_PER_DOLLAR
  const reconstructedState = useMemoState(events, playbackIndex, defaultInitialBudgetInTokens)

  // Helper to format values as Tokens or USD based on toggle
  const formatValue = (tokens) => {
    if (displayUnit === 'token') {
      return `${Math.round(tokens).toLocaleString()} tks`;
    } else {
      const usdValue = tokens / TOKENS_PER_DOLLAR;
      return `$${usdValue.toFixed(4)}`;
    }
  };

  // Compute advanced statistics of the swarm network
  const advancedStats = React.useMemo(() => {
    const nodes = reconstructedState.nodes;
    if (nodes.length === 0) return null;

    // 1. Structural levels of the swarm hierarchy
    const level0Agents = nodes.filter(n => n.Role?.toUpperCase() === 'ARCHITECT');
    const level1Agents = nodes.filter(n => n.Role?.toUpperCase() === 'POSTIER');
    const level2Agents = nodes.filter(n => n.Role?.toUpperCase() !== 'ARCHITECT' && n.Role?.toUpperCase() !== 'POSTIER');

    const numLevels = [level0Agents, level1Agents, level2Agents].filter(arr => arr.length > 0).length;

    // 2. Financial & Token Economics
    const totalRemainingBudget = nodes.reduce((sum, n) => sum + (n.Budget || 0), 0);
    const architectNode = level0Agents[0];
    const initialSwarmBudget = architectNode ? (architectNode.InitialBudget || architectNode.Budget || 0) : defaultInitialBudgetInTokens;
    const swarmRemainingBudget = totalRemainingBudget; 
    const swarmConsumedBudget = Math.max(0, initialSwarmBudget - swarmRemainingBudget);
    
    const consumedPercent = initialSwarmBudget > 0 ? (swarmConsumedBudget / initialSwarmBudget) * 100 : 0;
    const remainingPercent = 100 - consumedPercent;

    // Consumption breakdown by roles
    const computeConsumed = (agent) => Math.max(0, (agent.InitialBudget || agent.Budget || 0) - (agent.Budget || 0));
    const consumedByArchitect = level0Agents.reduce((sum, n) => sum + computeConsumed(n), 0);
    const consumedByPostiers = level1Agents.reduce((sum, n) => sum + computeConsumed(n), 0);
    const consumedByWorkers = level2Agents.reduce((sum, n) => sum + computeConsumed(n), 0);

    // 3. Structural Link Connections in Graph
    const linkCounts = {};
    nodes.forEach(n => { linkCounts[n.ID] = 0; });

    nodes.forEach(node => {
      if (node.PostierID && linkCounts[node.PostierID] !== undefined) {
        linkCounts[node.ID]++;
        linkCounts[node.PostierID]++;
      } else if (node.BubbleID) {
        const bubbleNodes = nodes.filter(n => n.BubbleID === node.BubbleID);
        const postier = bubbleNodes.find(n => n.Role?.toUpperCase() === 'POSTIER');
        const architect = nodes.find(n => n.Role?.toUpperCase() === 'ARCHITECT');

        if (node.Role?.toUpperCase() === 'POSTIER' && architect) {
          linkCounts[architect.ID]++;
          linkCounts[node.ID]++;
        } else if (node.Role?.toUpperCase() === 'RESUMEUR' && postier) {
          linkCounts[postier.ID]++;
          linkCounts[node.ID]++;
        } else if (!postier && architect && node.Role?.toUpperCase() !== 'ARCHITECT') {
          linkCounts[architect.ID]++;
          linkCounts[node.ID]++;
        }
      }
    });

    // Average links count metrics
    const decisionMakers = [...level0Agents, ...level1Agents];
    const totalDecisionLinks = decisionMakers.reduce((sum, n) => sum + (linkCounts[n.ID] || 0), 0);
    const avgLinksDecisionMakers = decisionMakers.length > 0 ? totalDecisionLinks / decisionMakers.length : 0;

    const avgLinksLevel0 = level0Agents.length > 0 ? level0Agents.reduce((sum, n) => sum + (linkCounts[n.ID] || 0), 0) / level0Agents.length : 0;
    const avgLinksLevel1 = level1Agents.length > 0 ? level1Agents.reduce((sum, n) => sum + (linkCounts[n.ID] || 0), 0) / level1Agents.length : 0;
    const avgLinksLevel2 = level2Agents.length > 0 ? level2Agents.reduce((sum, n) => sum + (linkCounts[n.ID] || 0), 0) / level2Agents.length : 0;

    return {
      level0Agents,
      level1Agents,
      level2Agents,
      numLevels,
      initialSwarmBudget,
      swarmRemainingBudget,
      swarmConsumedBudget,
      consumedPercent,
      remainingPercent,
      consumedByArchitect,
      consumedByPostiers,
      consumedByWorkers,
      linkCounts,
      avgLinksDecisionMakers,
      avgLinksLevel0,
      avgLinksLevel1,
      avgLinksLevel2
    };
  }, [reconstructedState.nodes, defaultInitialBudgetInTokens]);

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
            <span className="text-[10px] text-[var(--text-muted)] font-mono font-medium tracking-tight flex items-center gap-2">
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
              <button 
                type="button"
                onClick={() => setShowDiagnostics(!showDiagnostics)}
                className="ml-2 text-[9px] text-blue-400 font-bold bg-blue-500/10 px-1.5 py-0.5 rounded border border-blue-500/20 hover:bg-blue-500/20 transition-all cursor-pointer font-mono"
              >
                📊 DIAGNOSTIC
              </button>
              
              <button 
                type="button"
                onClick={handleResetSession}
                className="ml-2 text-[9px] text-red-400 font-bold bg-red-500/10 px-1.5 py-0.5 rounded border border-red-500/20 hover:bg-red-500/20 transition-all cursor-pointer font-mono"
                title="Réinitialiser la session et lancer une nouvelle mission"
              >
                🔄 NOUVELLE MISSION
              </button>

              {reconstructedState.nodes.length > 0 && (
                <button 
                  type="button"
                  onClick={() => setShowStats(!showStats)}
                  className="ml-2 text-[9px] text-purple-400 font-bold bg-purple-500/10 px-1.5 py-0.5 rounded border border-purple-500/20 hover:bg-purple-500/20 transition-all cursor-pointer font-mono"
                  title="Ouvrir le cockpit analytique et statistiques de l'essaim"
                >
                  📈 STATISTIQUES
                </button>
              )}

              {reconstructedState.nodes.length > 0 && (
                <button 
                  type="button"
                  onClick={() => setDisplayUnit(prev => prev === 'token' ? 'usd' : 'token')}
                  className="ml-2 text-[9px] text-green-400 font-bold bg-green-500/10 px-1.5 py-0.5 rounded border border-green-500/20 hover:bg-green-500/20 hover:border-green-500/40 transition-all cursor-pointer font-mono"
                  title="Basculer l'unité d'affichage entre Tokens (tks) et Dollars ($)"
                >
                  💵 UNITÉ : {displayUnit === 'token' ? 'TOKENS' : 'USD ($)'}
                </button>
              )}

              {reconstructedState.nodes.length > 0 && (
                <button 
                  type="button"
                  onClick={handleStopSwarm}
                  className="ml-2 text-[9px] text-orange-400 font-bold bg-orange-500/10 px-1.5 py-0.5 rounded border border-orange-500/20 hover:bg-orange-500/20 hover:border-orange-500/40 transition-all cursor-pointer font-mono"
                  title="Arrêter immédiatement tout le réseau d'agents"
                >
                  🛑 ARRÊTER L'ESSAIM
                </button>
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
                <div className="flex justify-between items-center border-b border-[var(--border-color)] pb-4">
                  <div className="flex items-center gap-3">
                    <Sparkles size={24} className="text-blue-400 animate-pulse" />
                    <div>
                      <h2 className="text-lg font-bold text-white tracking-wide uppercase">Lancer une nouvelle Mission Alpha</h2>
                      <p className="text-xs text-[var(--text-muted)]">Configurez l'objectif global et l'enveloppe budgétaire de l'essaim.</p>
                    </div>
                  </div>
                  
                  {wsStatus === 'connected' && (
                    <div className="flex items-center gap-2">
                      {apiTestStatus === 'testing' && (
                        <span className="text-[10px] text-yellow-400 font-semibold bg-yellow-500/10 px-2.5 py-1 rounded-full border border-yellow-500/20 flex items-center gap-1.5 animate-pulse">
                          <Activity size={10} className="animate-spin" />
                          Diagnostic API...
                        </span>
                      )}
                      {apiTestStatus === 'success' && (
                        <span className="text-[10px] text-green-400 font-semibold bg-green-500/10 px-2.5 py-1 rounded-full border border-green-500/20 flex items-center gap-1">
                          ✅ Clé API Active
                        </span>
                      )}
                      {apiTestStatus === 'error' && (
                        <button 
                          type="button"
                          onClick={handleTestAPI}
                          className="text-[10px] text-red-400 font-semibold bg-red-500/10 px-2.5 py-1 rounded-full border border-red-500/20 flex items-center gap-1 hover:bg-red-500/20 transition-all cursor-pointer"
                          title="Cliquez pour retester la clé API"
                        >
                          ❌ API Invalide (Retester)
                        </button>
                      )}
                    </div>
                  )}
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
                      <label className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider">Budget Initial Max ($)</label>
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
                        <strong className="text-[var(--text-secondary)] font-bold">{Math.round((budgetInput || 0) * TOKENS_PER_DOLLAR).toLocaleString()} tokens</strong> Gemini.
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

                  {wsStatus === 'connected' && apiTestStatus === 'error' && (
                    <div className="glass-panel p-3 flex gap-3 items-center" style={{ borderColor: 'rgba(255, 23, 68, 0.25)', background: 'rgba(255, 23, 68, 0.05)' }}>
                      <AlertTriangle size={18} className="text-red-400" />
                      <div className="flex flex-col">
                        <span className="text-xs font-bold text-red-400 uppercase">Clé API Gemini Invalide</span>
                        <span className="text-[10px] text-[var(--text-muted)] leading-tight">
                          La clé API Gemini configurée dans le fichier <code>.env</code> est invalide ou rejetée. Erreur : <span className="text-red-300 font-semibold">{apiTestResult}</span>.
                        </span>
                      </div>
                    </div>
                  )}

                  <button
                    type="submit"
                    disabled={wsStatus !== 'connected' || apiTestStatus !== 'success'}
                    className="mt-2 bg-blue-500 hover:bg-blue-600 text-white font-bold py-3 px-6 rounded-lg text-sm uppercase tracking-wider flex items-center justify-center gap-2 transition-all transform active:scale-95 disabled:opacity-40 disabled:hover:bg-blue-500 disabled:active:scale-100 disabled:cursor-not-allowed"
                    style={{
                      boxShadow: wsStatus === 'connected' && apiTestStatus === 'success' ? '0 0 15px rgba(59,130,246,0.3)' : 'none'
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
            <div className="flex-1 h-full relative flex items-center justify-center" style={{ minHeight: '400px' }}>
              <SwarmGraph 
                nodes={reconstructedState.nodes}
                activeAgentId={activeSelectedAgent?.ID}
                onSelectAgent={setSelectedAgent}
                displayUnit={displayUnit}
              />
              
              {/* Floating current objective bar */}
              <div className="absolute top-6 max-w-lg glass-panel px-4 py-2 flex items-center gap-3" style={{ left: '190px' }}>
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
            <div className="border-t border-[var(--border-color)] bg-[rgba(7,9,14,0.65)] backdrop-filter var(--glass-blur) flex flex-col flex-shrink-0 relative" style={{ height: `${logPanelHeight}px` }}>
              {/* Resizer drag handle bar */}
              <div 
                className="absolute top-0 left-0 right-0 h-1.5 bg-white/5 hover:bg-blue-500/50 cursor-ns-resize transition-all z-20"
                onMouseDown={handleMouseDownResize}
                title="Glisser verticalement pour redimensionner le flux tactile"
              />
              <div className="flex justify-between items-center px-4 py-2 border-b border-[var(--border-color)] pt-2.5">
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
            {activeSelectedAgent && (() => {
              const initialAllocatedBudget = activeSelectedAgent.InitialBudget || activeSelectedAgent.Budget || 0;
              const currentBudget = activeSelectedAgent.Budget || 0;
              const tokensConsumed = Math.max(0, initialAllocatedBudget - currentBudget);
              const costConsumedUSD = tokensConsumed / TOKENS_PER_DOLLAR;

              return (
                <div className="p-6 flex flex-col h-full gap-5">
                  {/* Header Inspector */}
                  <div className="flex justify-between items-start border-b border-[var(--border-color)] pb-4 flex-shrink-0">
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

                  {/* Main scrollable body area */}
                  <div className="flex-1 overflow-y-auto flex flex-col gap-5 pr-1 custom-scrollbar">
                    {/* Status & Allocated Budget Row */}
                    <div className="grid grid-cols-2 gap-4 flex-shrink-0">
                      <div className="glass-panel p-3 flex flex-col" style={{ background: 'rgba(7, 9, 14, 0.4)' }}>
                        <span className="text-[9px] text-[var(--text-muted)] uppercase font-semibold">Statut</span>
                        <span className={`text-xs font-bold uppercase mt-1 ${activeSelectedAgent.Status === 'dead' ? 'text-red-400' : activeSelectedAgent.Status === 'working' ? 'text-green-400 animate-pulse' : 'text-blue-400'}`}>
                          {activeSelectedAgent.Status || 'INCONNU'}
                        </span>
                      </div>
                      <div className="glass-panel p-3 flex flex-col" style={{ background: 'rgba(7, 9, 14, 0.4)' }}>
                        <span className="text-[9px] text-[var(--text-muted)] uppercase font-semibold">Enveloppe Jetons</span>
                        <span className="text-xs font-bold text-white mt-1 font-mono">
                          {displayUnit === 'token' ? (
                            `${currentBudget.toLocaleString(undefined, { maximumFractionDigits: 0 })} tks`
                          ) : (
                            `$${(currentBudget / TOKENS_PER_DOLLAR).toFixed(4)}`
                          )}
                        </span>
                      </div>
                    </div>

                    {/* Token details panel (detailed economics) */}
                    <div className="glass-panel p-3.5 flex flex-col gap-2 flex-shrink-0 font-sans" style={{ background: 'rgba(7, 9, 14, 0.25)' }}>
                      <span className="text-[9px] text-[var(--text-muted)] uppercase font-bold tracking-wider mb-1">Économie de l'Agent</span>
                      
                      <div className="flex justify-between items-center text-xs font-mono">
                        <span className="text-[var(--text-muted)]">Budget Initial :</span>
                        <span className="text-slate-300 font-semibold">{initialAllocatedBudget.toLocaleString()} tks</span>
                      </div>
                      
                      <div className="flex justify-between items-center text-xs font-mono">
                        <span className="text-[var(--text-muted)]">Jetons Restants :</span>
                        <span className="text-slate-300 font-semibold">{currentBudget.toLocaleString()} tks</span>
                      </div>

                      <div className="border-t border-[var(--border-color)] my-1"></div>

                      <div className="flex justify-between items-center text-xs font-mono">
                        <span className="text-green-400 font-semibold">Tokens Consommés :</span>
                        <span className="text-green-400 font-bold">{tokensConsumed.toLocaleString()} tks</span>
                      </div>
                      
                      <div className="flex justify-between items-center text-xs font-mono">
                        <span className="text-green-400 font-semibold">Coût Consommé ($) :</span>
                        <span className="text-green-400 font-bold">${costConsumedUSD.toFixed(6)}</span>
                      </div>
                    </div>

                    {/* Connection Details (Bubble and Postier Full IDs) */}
                    <div className="flex flex-col gap-3 flex-shrink-0">
                      {activeSelectedAgent.BubbleID && (
                        <div className="glass-panel p-3 flex flex-col gap-1.5" style={{ background: 'rgba(189, 0, 255, 0.03)', borderColor: 'rgba(189, 0, 255, 0.15)' }}>
                          <span className="text-[var(--text-muted)] text-[9px] uppercase font-bold tracking-wider">Bulle de rattachement (ID complet)</span>
                          <span className="font-mono text-purple-400 text-[10px] font-semibold break-all select-all">{activeSelectedAgent.BubbleID}</span>
                        </div>
                      )}

                      {activeSelectedAgent.PostierID && (
                        <div className="glass-panel p-3 flex flex-col gap-1.5" style={{ background: 'rgba(59, 130, 246, 0.03)', borderColor: 'rgba(59, 130, 246, 0.15)' }}>
                          <span className="text-[var(--text-muted)] text-[9px] uppercase font-bold tracking-wider">Postier de rattachement (Broker)</span>
                          <span className="font-mono text-blue-400 text-[10px] font-semibold break-all select-all">{activeSelectedAgent.PostierID}</span>
                        </div>
                      )}
                    </div>

                    {/* Task Instructions */}
                    <div className="flex flex-col gap-2 flex-shrink-0">
                      <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] tracking-wider">Instruction de Tâche</span>
                      <div className="glass-panel p-3 text-xs leading-relaxed text-[var(--text-secondary)] font-mono bg-[rgba(7,9,14,0.3)] select-text break-words whitespace-pre-wrap">
                        {activeSelectedAgent.TaskDescription || (
                          <span className="text-[var(--text-muted)] italic">Aucune instruction directe stockée (Agent de planification).</span>
                        )}
                      </div>
                    </div>

                    {/* System & Metadata (Retry count, subtasks pending, Created at) */}
                    <div className="glass-panel p-3 flex flex-col gap-2 font-mono text-[11px] flex-shrink-0" style={{ background: 'rgba(7, 9, 14, 0.2)' }}>
                      <span className="text-[9px] text-[var(--text-muted)] uppercase font-bold tracking-wider mb-1">Métadonnées Système</span>
                      <div className="flex justify-between">
                        <span className="text-[var(--text-muted)]">Sous-tâches en attente :</span>
                        <span className="text-slate-300 font-semibold">{activeSelectedAgent.SubtasksPending ?? 0}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-[var(--text-muted)]">Nombre de tentatives (Retries) :</span>
                        <span className="text-slate-300 font-semibold">{activeSelectedAgent.RetryCount ?? 0}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-[var(--text-muted)]">Nombre de liaisons physiques :</span>
                        <span className="text-slate-300 font-semibold">{advancedStats?.linkCounts[activeSelectedAgent.ID] ?? 0}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-[var(--text-muted)]">Date de création :</span>
                        <span className="text-slate-300 font-semibold">
                          {activeSelectedAgent.CreatedAt ? new Date(activeSelectedAgent.CreatedAt).toLocaleTimeString() : 'N/A'}
                        </span>
                      </div>
                    </div>

                    {/* Agent Memory Log */}
                    {activeSelectedAgent.Memory && activeSelectedAgent.Memory.length > 0 && (
                      <div className="flex flex-col gap-2 flex-shrink-0">
                        <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] tracking-wider">Mémoire Contextuelle ({activeSelectedAgent.Memory.length} messages)</span>
                        <div className="glass-panel p-3 text-[11px] leading-relaxed text-[var(--text-secondary)] font-mono max-h-60 overflow-y-auto bg-[rgba(7,9,14,0.4)] flex flex-col gap-3 custom-scrollbar">
                          {activeSelectedAgent.Memory.map((msg, index) => (
                            <div key={index} className="flex flex-col gap-1 border-b border-[var(--border-color)] pb-2 last:border-0 last:pb-0 select-text">
                              <span className={`text-[9px] uppercase font-bold tracking-widest ${msg.role === 'system' ? 'text-blue-400' : msg.role === 'assistant' ? 'text-purple-400' : 'text-green-400'}`}>
                                {msg.role}
                              </span>
                              <span className="text-slate-300 break-words whitespace-pre-wrap">{msg.content}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {/* Warn if bankrupt */}
                    {currentBudget <= 0 && (
                      <div className="glass-panel p-3 flex gap-3 items-center flex-shrink-0" style={{ borderColor: 'rgba(255, 23, 68, 0.2)', background: 'rgba(255, 23, 68, 0.05)' }}>
                        <AlertTriangle size={18} className="text-red-400 animate-bounce flex-shrink-0" />
                        <div className="flex flex-col">
                          <span className="text-xs font-bold text-red-400 uppercase">Banqueroute déclenchée</span>
                          <span className="text-[9px] text-[var(--text-muted)] leading-tight font-mono">Cet agent a épuisé son budget de jetons. Kill Switch activé.</span>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              )
            })()}
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

      {showStats && advancedStats && (
        <div className="absolute inset-0 z-50 bg-[#07090e]/95 backdrop-blur-md flex items-center justify-center p-6">
          <div className="glass-panel p-6 w-full max-w-4xl flex flex-col gap-5 relative" style={{ background: 'rgba(13, 17, 27, 0.95)', borderColor: 'var(--role-postier)' }}>
            <button 
              type="button"
              onClick={() => setShowStats(false)}
              className="absolute top-4 right-4 p-1.5 rounded-md border border-[var(--border-color)] text-[var(--text-muted)] hover:text-white hover:bg-[var(--bg-tertiary)] cursor-pointer z-10"
            >
              <X size={16} />
            </button>

            {/* Header section (strictly static) */}
            <div className="border-b border-[var(--border-color)] pb-3 flex justify-between items-center">
              <h3 className="text-sm font-bold text-white uppercase tracking-wider font-mono flex items-center gap-2">
                <Activity size={14} className="text-purple-400 animate-pulse" />
                Tableau de Bord Analytique de l'Essaim
              </h3>
              
              <div className="flex items-center gap-2 mr-8 font-mono text-[10px]">
                <span className="text-[var(--text-muted)]">Unité Active :</span>
                <span className="text-green-400 font-bold bg-green-500/10 px-2 py-0.5 rounded border border-green-500/20 uppercase">
                  {displayUnit === 'token' ? 'Tokens' : 'USD ($)'}
                </span>
              </div>
            </div>

            {/* Bulletproof scrollable body container with rigid CSS max-height constraint */}
            <div className="overflow-y-auto pr-1 flex flex-col gap-6 custom-scrollbar" style={{ maxHeight: '62vh' }}>
              {/* Donut charts grid - 2 Columns */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
                {/* Chart 1: Global Budget Usage Ratio */}
                <PremiumDonutChart
                  title="Consommation du Budget"
                  centerLabel="Consommé"
                  centerValue={`${advancedStats.consumedPercent.toFixed(0)}%`}
                  data={[
                    { label: 'Utilisé', value: advancedStats.swarmConsumedBudget, color: '#ef4444', colorName: 'Rouge' },
                    { label: 'Restant', value: advancedStats.swarmRemainingBudget, color: '#22c55e', colorName: 'Vert' }
                  ]}
                />

                {/* Chart 2: Consumption Share by Role */}
                <PremiumDonutChart
                  title="Consommation par Rôle"
                  centerLabel="Total Rôles"
                  centerValue={formatValue(advancedStats.swarmConsumedBudget)}
                  data={[
                    { label: 'Architecte', value: advancedStats.consumedByArchitect, color: '#3b82f6', colorName: 'Bleu' },
                    { label: 'Postiers', value: advancedStats.consumedByPostiers, color: '#ef4444', colorName: 'Rouge' },
                    { label: 'Exécutants', value: advancedStats.consumedByWorkers, color: '#22c55e', colorName: 'Vert' }
                  ]}
                />

                {/* Chart 3: Level Structural Distribution */}
                <PremiumDonutChart
                  title="Agents par Niveau"
                  centerLabel="Total Niveaux"
                  centerValue={`${advancedStats.numLevels} Niveaux`}
                  data={[
                    { label: 'Niv 0 (Stratégique)', value: advancedStats.level0Agents.length, color: '#3b82f6', colorName: 'Bleu' },
                    { label: 'Niv 1 (Logistique)', value: advancedStats.level1Agents.length, color: '#ef4444', colorName: 'Rouge' },
                    { label: 'Niv 2 (Exécution)', value: advancedStats.level2Agents.length, color: '#22c55e', colorName: 'Vert' }
                  ]}
                />
              </div>

              {/* Extra numerical table analytics */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-5 mt-2 text-xs font-mono">
                {/* Connectivity / Average Links Card */}
                <div className="glass-panel p-4 flex flex-col gap-2.5 bg-[rgba(7,9,14,0.3)]">
                  <span className="text-[10px] text-[var(--text-muted)] font-bold tracking-wider uppercase mb-1">Métriques de Liaison & Connectivité</span>
                  
                  <div className="flex justify-between items-center">
                    <span className="text-[var(--text-muted)]">Liaison moyenne par Décisionnaire :</span>
                    <span className="text-white font-bold">{advancedStats.avgLinksDecisionMakers.toFixed(2)} liaisons</span>
                  </div>
                  <div className="border-t border-[var(--border-color)] my-0.5"></div>
                  <div className="flex justify-between items-center">
                    <span className="text-[var(--text-muted)]">Moyenne Niveau 0 (Architecte) :</span>
                    <span className="text-slate-300 font-semibold">{advancedStats.avgLinksLevel0.toFixed(2)} liaisons</span>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-[var(--text-muted)]">Moyenne Niveau 1 (Postiers) :</span>
                    <span className="text-slate-300 font-semibold">{advancedStats.avgLinksLevel1.toFixed(2)} liaisons</span>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-[var(--text-muted)]">Moyenne Niveau 2 (Exécutants) :</span>
                    <span className="text-slate-300 font-semibold">{advancedStats.avgLinksLevel2.toFixed(2)} liaisons</span>
                  </div>
                </div>

                {/* Individual budgets detailed view */}
                <div className="glass-panel p-4 flex flex-col gap-2 bg-[rgba(7,9,14,0.3)]">
                  <span className="text-[10px] text-[var(--text-muted)] font-bold tracking-wider uppercase mb-1">Détails Économiques par Agent</span>
                  <div className="max-h-36 overflow-y-auto flex flex-col gap-1.5 pr-1 custom-scrollbar">
                    {reconstructedState.nodes.map(node => {
                      const nodeInit = node.InitialBudget || node.Budget || 0;
                      const nodeCurr = node.Budget || 0;
                      const nodeUsed = Math.max(0, nodeInit - nodeCurr);
                      const nodePercent = nodeInit > 0 ? (nodeUsed / nodeInit) * 100 : 0;
                      return (
                        <div key={node.ID} className="flex justify-between items-center text-[10px] border-b border-[var(--border-color)] pb-1 last:border-0">
                          <div className="flex items-center gap-1.5 truncate max-w-sm">
                            <span className="w-1.5 h-1.5 rounded-full flex-shrink-0" style={{ backgroundColor: getRoleColor(node.Role) }} />
                            <span className="text-slate-300 font-bold">{node.Role}</span>
                            <span className="text-[var(--text-muted)] font-mono">({node.ID.substring(0, 4)})</span>
                          </div>
                          <div className="flex items-center gap-2">
                            <span className="text-slate-300 font-mono font-medium">
                              {formatValue(nodeCurr)} / {formatValue(nodeInit)}
                            </span>
                            <span className="text-orange-400 font-bold font-mono text-right w-10">
                              {nodePercent.toFixed(0)}%
                            </span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {showDiagnostics && (
        <div className="absolute inset-0 z-50 bg-[#07090e]/95 backdrop-blur-md flex items-center justify-center p-6">
          <div className="glass-panel p-6 w-full max-w-2xl flex flex-col gap-4 relative" style={{ background: 'rgba(13, 17, 27, 0.95)', borderColor: 'var(--role-architect)' }}>
            <button 
              type="button"
              onClick={() => setShowDiagnostics(false)}
              className="absolute top-4 right-4 p-1.5 rounded-md border border-[var(--border-color)] text-[var(--text-muted)] hover:text-white hover:bg-[var(--bg-tertiary)] cursor-pointer"
            >
              <X size={16} />
            </button>

            <div className="border-b border-[var(--border-color)] pb-3">
              <h3 className="text-sm font-bold text-white uppercase tracking-wider font-mono flex items-center gap-2">
                <Sparkles size={14} className="text-blue-400 animate-pulse" />
                Console de Diagnostic ESSAIM-IA
              </h3>
            </div>

            <div className="grid grid-cols-2 gap-4 text-xs font-mono">
              <div className="glass-panel p-3 flex flex-col gap-1.5" style={{ background: 'rgba(7, 9, 14, 0.4)' }}>
                <span className="text-[10px] text-[var(--text-muted)] uppercase font-semibold">Statut WebSocket</span>
                <span className={`font-bold uppercase ${wsStatus === 'connected' ? 'text-green-400' : 'text-red-400'}`}>
                  {wsStatus}
                </span>
              </div>
              <div className="glass-panel p-3 flex flex-col gap-1.5" style={{ background: 'rgba(7, 9, 14, 0.4)' }}>
                <span className="text-[10px] text-[var(--text-muted)] uppercase font-semibold">Diagnostic Clé API</span>
                <span className={`font-bold uppercase ${apiTestStatus === 'success' ? 'text-green-400' : apiTestStatus === 'error' ? 'text-red-400' : 'text-yellow-400'}`}>
                  {apiTestStatus} {apiTestStatus === 'success' ? '✅' : ''}
                </span>
              </div>
            </div>

            <div className="flex flex-col gap-2">
              <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] tracking-wider font-mono">Statistiques des Événements Reçus</span>
              <div className="glass-panel p-3 text-xs font-mono grid grid-cols-2 gap-3 bg-[rgba(7,9,14,0.3)]">
                <div>Total Événements : <span className="text-white font-bold">{events.length}</span></div>
                <div>Index Actuel : <span className="text-white font-bold">{playbackIndex}</span></div>
                <div>GRAPH_UPDATE : <span className="text-blue-400 font-bold">{events.filter(e => e.type === 'GRAPH_UPDATE').length}</span></div>
                <div>AGENT_STATE : <span className="text-purple-400 font-bold">{events.filter(e => e.type === 'AGENT_STATE').length}</span></div>
                <div>LOG_STREAM : <span className="text-green-400 font-bold">{events.filter(e => e.type === 'LOG_STREAM').length}</span></div>
                <div>SYSTEM_ALERT : <span className="text-red-400 font-bold">{events.filter(e => e.type === 'SYSTEM_ALERT').length}</span></div>
              </div>
            </div>

            <div className="flex flex-col gap-2 flex-1 min-h-[150px]">
              <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] tracking-wider font-mono">Dernier Événement Reçu</span>
              <pre className="glass-panel p-3 text-[11px] leading-relaxed text-slate-300 overflow-auto flex-1 bg-[rgba(7,9,14,0.35)]" style={{ maxHeight: '180px' }}>
                {events.length > 0 ? (
                  JSON.stringify(events[events.length - 1], null, 2)
                ) : (
                  <span className="text-[var(--text-muted)] italic">Aucun événement reçu pour le moment...</span>
                )}
              </pre>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ==========================================================================
// STATE RECONSTRUCTION HOOK/HELPER (THE TIME-TRAVEL ENGINE)
// ==========================================================================
function useMemoState(events, playbackIndex, defaultInitialBudget) {
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
            status: (rawAgent.status || rawAgent.Status || 'born').toLowerCase().replace('status_', ''),
            Status: (rawAgent.status || rawAgent.Status || 'born').toLowerCase().replace('status_', ''),
            budget: rawAgent.budget !== undefined ? rawAgent.budget : rawAgent.Budget,
            Budget: rawAgent.budget !== undefined ? rawAgent.budget : rawAgent.Budget,
            initialBudget: rawAgent.budget !== undefined ? rawAgent.budget : rawAgent.Budget,
            InitialBudget: rawAgent.budget !== undefined ? rawAgent.budget : rawAgent.Budget,
            retryCount: rawAgent.retryCount !== undefined ? rawAgent.retryCount : (rawAgent.RetryCount || 0),
            RetryCount: rawAgent.retryCount !== undefined ? rawAgent.retryCount : (rawAgent.RetryCount || 0),
            subtasksPending: rawAgent.subtasksPending !== undefined ? rawAgent.subtasksPending : (rawAgent.SubtasksPending || 0),
            SubtasksPending: rawAgent.subtasksPending !== undefined ? rawAgent.subtasksPending : (rawAgent.SubtasksPending || 0),
            createdAt: rawAgent.createdAt || rawAgent.CreatedAt,
            CreatedAt: rawAgent.createdAt || rawAgent.CreatedAt,
            memory: rawAgent.memory || rawAgent.Memory || [],
            Memory: rawAgent.memory || rawAgent.Memory || [],
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
            node.status = status.toLowerCase().replace('status_', '')
            node.Status = status.toLowerCase().replace('status_', '')
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

    // Dynamically retrieve initial budget from the Architect's birth event to support any custom budget input scale
    const architectEvent = events.find(ev => ev.type === 'GRAPH_UPDATE' && ev.payload?.action === 'ADD_NODE' && (ev.payload?.agent?.Role === 'ARCHITECT' || ev.payload?.agent?.role === 'ARCHITECT'))
    const initialBudget = architectEvent?.payload?.agent?.budget || architectEvent?.payload?.agent?.Budget || defaultInitialBudget
    
    const currentBudget = nodes.reduce((sum, n) => sum + (n.budget || n.Budget || 0), 0)
    
    totalTokens = nodes.length > 0 ? Math.max(0, initialBudget - currentBudget) : 0
    
    // Unify mathematically: Cost in Dollars calculated directly from tokens
    const totalCost = totalTokens / TOKENS_PER_DOLLAR

    return {
      nodes,
      logs,
      totalTokens,
      totalCost,
      objective
    }
  }, [events, playbackIndex, defaultInitialBudget])
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

// ==========================================================================
// PURE SVG PREMIUM CIRCULAR DONUT CHART COMPONENT
// ==========================================================================
function PremiumDonutChart({ data, title, centerLabel, centerValue }) {
  const total = data.reduce((sum, item) => sum + item.value, 0) || 1;
  const radius = 38;
  const strokeWidth = 8;
  const circumference = 2 * Math.PI * radius; // ~238.76

  let accumulatedPercent = 0;

  return (
    <div className="glass-panel p-3 flex flex-col items-center gap-2 select-none" style={{ background: 'rgba(7, 9, 14, 0.3)', minWidth: '150px' }}>
      <span className="text-[9px] text-[var(--text-muted)] font-bold tracking-wider uppercase">{title}</span>
      <div className="relative flex items-center justify-center" style={{ width: '115px', height: '115px' }}>
        <svg width="100%" height="100%" viewBox="0 0 100 100" className="transform -rotate-90">
          {/* Base empty ring */}
          <circle
            cx="50"
            cy="50"
            r={radius}
            fill="transparent"
            stroke="rgba(255, 255, 255, 0.02)"
            strokeWidth={strokeWidth}
          />
          {/* Slices */}
          {data.map((item, idx) => {
            const percent = item.value / total;
            const strokeDasharray = `${percent * circumference} ${circumference}`;
            const strokeDashoffset = -accumulatedPercent * circumference;
            accumulatedPercent += percent;

            if (item.value === 0) return null;

            return (
              <circle
                key={idx}
                cx="50"
                cy="50"
                r={radius}
                fill="transparent"
                stroke={item.color}
                strokeWidth={strokeWidth}
                strokeDasharray={strokeDasharray}
                strokeDashoffset={strokeDashoffset}
                strokeLinecap="round"
                className="transition-all duration-300 ease-in-out hover:opacity-85"
                style={{ cursor: 'pointer' }}
                title={`${item.label}: ${item.value}`}
              />
            );
          })}
        </svg>
        {/* Center label */}
        <div className="absolute flex flex-col items-center justify-center text-center px-1">
          <span className="text-[9px] uppercase font-bold text-[var(--text-muted)] tracking-wider leading-tight">{centerLabel}</span>
          <span className="text-[13px] font-bold text-white font-mono mt-0.5">{centerValue}</span>
        </div>
      </div>
      {/* Legend list */}
      <div className="w-full flex flex-col gap-1.5 mt-1 font-mono text-[9px]">
        {data.map((item, idx) => {
          const percent = total > 0 ? (item.value / total) * 100 : 0;
          return (
            <div key={idx} className="flex justify-between items-center gap-2">
              <div className="flex items-center gap-1.5 truncate">
                <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: item.color }} />
                <span className="truncate font-bold" style={{ color: item.color }} title={item.label}>
                  {item.label} {item.colorName ? `(${item.colorName})` : ''}
                </span>
              </div>
              <span className="font-bold flex-shrink-0 text-right w-12" style={{ color: item.color }}>
                {percent.toFixed(0)}%
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
