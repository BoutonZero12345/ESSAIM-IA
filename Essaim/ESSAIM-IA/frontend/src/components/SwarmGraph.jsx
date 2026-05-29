import React, { useMemo } from 'react'

export default function SwarmGraph({ nodes, activeAgentId, onSelectAgent }) {
  const width = 800
  const height = 500

  // Calculate layout coordinates for nodes dynamically
  const positionedNodes = useMemo(() => {
    const layout = {}
    
    // 1. Separate nodes by bubble
    const rootNodes = nodes.filter(n => !n.BubbleID)
    const bubbleIds = [...new Set(nodes.filter(n => n.BubbleID).map(n => n.BubbleID))]
    
    // Place root nodes (Architect, etc.) at the top
    rootNodes.forEach((node, idx) => {
      layout[node.ID] = {
        ...node,
        x: width / 2 + (idx - (rootNodes.length - 1) / 2) * 160,
        y: 80
      }
    })

    // Place bubbles
    bubbleIds.forEach((bubbleId, bIdx) => {
      const bubbleNodes = nodes.filter(n => n.BubbleID === bubbleId)
      
      // Determine center of this bubble
      const bx = width / 2 + (bIdx - (bubbleIds.length - 1) / 2) * 260
      const by = 280

      // Find Postier if any
      const postier = bubbleNodes.find(n => n.Role === 'POSTIER')
      const workers = bubbleNodes.filter(n => n.Role !== 'POSTIER' && n.Role !== 'RESUMEUR')
      const resumeurs = bubbleNodes.filter(n => n.Role === 'RESUMEUR')

      // Position Postier in center
      if (postier) {
        layout[postier.ID] = {
          ...postier,
          x: bx,
          y: by
        }
      }

      // Position Workers in a circle around the center
      const radius = 80
      workers.forEach((worker, wIdx) => {
        const angle = (wIdx * 2 * Math.PI) / (workers.length || 1) - Math.PI / 2
        layout[worker.ID] = {
          ...worker,
          x: bx + radius * Math.cos(angle),
          y: by + radius * Math.sin(angle)
        }
      })

      // Position Résumeurs slightly outer, near the parent link
      resumeurs.forEach((resumeur, rIdx) => {
        const angle = -Math.PI / 2 + (rIdx - (resumeurs.length - 1) / 2) * 0.5
        layout[resumeur.ID] = {
          ...resumeur,
          x: bx + (radius * 1.3) * Math.cos(angle),
          y: by + (radius * 0.6) * Math.sin(angle)
        }
      })

      // Fallback if no Postier is found but there are workers, place first worker at center or lay out circularly
      if (!postier && bubbleNodes.length > 0) {
        bubbleNodes.forEach((node, idx) => {
          const angle = (idx * 2 * Math.PI) / bubbleNodes.length
          layout[node.ID] = {
            ...node,
            x: bx + 60 * Math.cos(angle),
            y: by + 60 * Math.sin(angle)
          }
        })
      }
    })

    return layout
  }, [nodes])

  // Generate links between nodes
  const links = useMemo(() => {
    const list = []
    const nodeMap = positionedNodes

    nodes.forEach(node => {
      // Worker connects to Postier if they have one
      if (node.PostierID && nodeMap[node.PostierID] && nodeMap[node.ID]) {
        list.push({
          id: `${node.ID}-${node.PostierID}`,
          from: nodeMap[node.ID],
          to: nodeMap[node.PostierID],
          type: 'worker-postier'
        })
      } 
      // Bubble nodes connect to their Parent Agent if no Postier
      else if (node.BubbleID && nodeMap[node.ID]) {
        // Find bubble's parent or connect directly to Architect
        const bubbleNodes = nodes.filter(n => n.BubbleID === node.BubbleID)
        const postier = bubbleNodes.find(n => n.Role === 'POSTIER')
        const architect = nodes.find(n => n.Role === 'ARCHITECT')

        if (node.Role === 'POSTIER' && architect && nodeMap[architect.ID]) {
          list.push({
            id: `${architect.ID}-${node.ID}`,
            from: nodeMap[architect.ID],
            to: nodeMap[node.ID],
            type: 'architect-postier'
          })
        } else if (node.Role === 'RESUMEUR' && postier && nodeMap[postier.ID]) {
          list.push({
            id: `${postier.ID}-${node.ID}`,
            from: nodeMap[postier.ID],
            to: nodeMap[node.ID],
            type: 'postier-resumeur'
          })
        } else if (!postier && architect && nodeMap[architect.ID] && node.Role !== 'ARCHITECT') {
          list.push({
            id: `${architect.ID}-${node.ID}`,
            from: nodeMap[architect.ID],
            to: nodeMap[node.ID],
            type: 'direct-parent'
          })
        }
      }
    })

    return list
  }, [nodes, positionedNodes])

  const getRoleColor = (role) => {
    switch (role?.toUpperCase()) {
      case 'ARCHITECT': return 'var(--role-architect)'
      case 'POSTIER': return 'var(--role-postier)'
      case 'RESUMEUR': return 'var(--role-resumeur)'
      case 'CRITIQUE': return 'var(--role-critique)'
      default: return 'var(--role-worker)'
    }
  }

  const getRoleLabel = (role) => {
    switch (role?.toUpperCase()) {
      case 'ARCHITECT': return 'ARCHI'
      case 'POSTIER': return 'POSTE'
      case 'RESUMEUR': return 'RÉSUM'
      case 'CRITIQUE': return 'CRITQ'
      default: return 'WORK'
    }
  }

  return (
    <div className="w-full h-full relative overflow-hidden flex items-center justify-center">
      {nodes.length === 0 ? (
        <div className="flex flex-col items-center gap-2 text-[var(--text-muted)]">
          <span className="text-4xl animate-bounce">🐝</span>
          <span className="text-sm font-semibold tracking-wider uppercase">En attente du lancement de la mission Alpha...</span>
        </div>
      ) : (
        <svg 
          viewBox={`0 0 ${width} ${height}`} 
          className="w-full h-full max-w-[90%] max-h-[90%]"
        >
          {/* Defs for gradients, filters, markers */}
          <defs>
            <filter id="glow" x="-20%" y="-20%" width="140%" height="140%">
              <feGaussianBlur stdDeviation="6" result="blur" />
              <feComposite in="SourceGraphic" in2="blur" operator="over" />
            </filter>
          </defs>

          {/* Links */}
          <g>
            {links.map(link => (
              <g key={link.id}>
                {/* Background glow link */}
                <line
                  x1={link.from.x}
                  y1={link.from.y}
                  x2={link.to.x}
                  y2={link.to.y}
                  className="link-line"
                  style={{ stroke: getRoleColor(link.from.Role), opacity: 0.15 }}
                />
                {/* Base link line */}
                <line
                  x1={link.from.x}
                  y1={link.from.y}
                  x2={link.to.x}
                  y2={link.to.y}
                  className="link-line"
                />
                
                {/* Packet flow particles */}
                <circle r="3" className="packet-particle" style={{ '--packet-color': getRoleColor(link.from.Role) }}>
                  <animateMotion
                    dur="3s"
                    repeatCount="indefinite"
                    path={`M ${link.from.x} ${link.from.y} L ${link.to.x} ${link.to.y}`}
                  />
                </circle>
              </g>
            ))}
          </g>

          {/* Nodes */}
          <g>
            {Object.values(positionedNodes).map(node => {
              const color = getRoleColor(node.Role)
              const isActive = node.Status === 'working' || node.Status === 'born' || activeAgentId === node.ID
              const isSelected = activeAgentId === node.ID

              return (
                <g 
                  key={node.ID} 
                  transform={`translate(${node.x}, ${node.y})`}
                  onClick={() => onSelectAgent(node)}
                  className="cursor-pointer"
                >
                  {/* Outer glow aura for active/selected nodes */}
                  {isActive && (
                    <circle
                      r="26"
                      fill="none"
                      stroke={color}
                      strokeWidth="2"
                      opacity="0.3"
                      className="node-outer node-active"
                      style={{ '--pulse-color': color }}
                    />
                  )}

                  {/* Outer border ring */}
                  <circle
                    r="20"
                    fill="var(--bg-secondary)"
                    stroke={isSelected ? '#ffffff' : color}
                    strokeWidth={isSelected ? 3 : 1.5}
                    className="node-inner"
                    style={{ filter: isSelected ? 'drop-shadow(0 0 8px ' + color + ')' : '' }}
                  />

                  {/* Tiny center core dot */}
                  <circle
                    r="4"
                    fill={color}
                  />

                  {/* Role text label */}
                  <text
                    y="32"
                    textAnchor="middle"
                    fill="var(--text-secondary)"
                    fontSize="9"
                    fontWeight="700"
                    letterSpacing="0.05em"
                    className="select-none font-mono"
                  >
                    {getRoleLabel(node.Role)}
                  </text>

                  {/* ID badge (first 4 chars) */}
                  <text
                    y="-28"
                    textAnchor="middle"
                    fill="var(--text-muted)"
                    fontSize="8"
                    fontFamily="monospace"
                    className="select-none"
                  >
                    {(node.ID || node.id || '').substring(0, 4)}
                  </text>
                </g>
              )
            })}
          </g>
        </svg>
      )}
    </div>
  )
}
