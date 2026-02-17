import { useRef, useCallback, useEffect } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import useStore from '../../store/useStore';

// Role color mapping per Architecture §10.1
const ROLE_COLORS = {
    ARCHITECT: '#3b82f6', // Blue
    WORKER: '#10b981',    // Green
    CRITIC: '#ef4444',    // Red
    CODER: '#8b5cf6',     // Purple
};

// Status shape per Architecture §10.1
const STATUS_DEAD = 'STATUS_DEAD';

/**
 * NetworkGraph - 2D force-directed graph visualization using react-force-graph-2d.
 * Nodes: Color by Role, Size by Budget, Shape by status.
 * Links: Animated particles representing JSON data flow.
 * Per Architecture §10.1.
 */
export default function NetworkGraph() {
    const graphRef = useRef();
    const { getGraphData, setSelectedAgent } = useStore();
    const graphData = getGraphData();

    // Custom node rendering
    const paintNode = useCallback((node, ctx) => {
        const size = Math.max(4, Math.min(16, (node.budget || 1) * 2));
        const color = ROLE_COLORS[node.role] || '#6b7280';

        ctx.beginPath();

        if (node.status === STATUS_DEAD) {
            // Dead = Cross shape
            ctx.moveTo(node.x - size, node.y - size);
            ctx.lineTo(node.x + size, node.y + size);
            ctx.moveTo(node.x + size, node.y - size);
            ctx.lineTo(node.x - size, node.y + size);
            ctx.strokeStyle = '#4b5563';
            ctx.lineWidth = 2;
            ctx.stroke();
        } else if (node.status === 'STATUS_REVIEW' || node.status === 'STATUS_WAITING') {
            // Waiting/Review = Square
            ctx.rect(node.x - size, node.y - size, size * 2, size * 2);
            ctx.fillStyle = color;
            ctx.fill();
        } else {
            // Active = Circle (Born, Working)
            ctx.arc(node.x, node.y, size, 0, 2 * Math.PI);
            ctx.fillStyle = color;
            ctx.fill();
        }

        // Glow effect for working agents
        if (node.status === 'STATUS_WORKING') {
            ctx.beginPath();
            ctx.arc(node.x, node.y, size + 4, 0, 2 * Math.PI);
            ctx.strokeStyle = color;
            ctx.lineWidth = 1.5;
            ctx.globalAlpha = 0.4;
            ctx.stroke();
            ctx.globalAlpha = 1;
        }

        // Label
        ctx.font = '3px Inter, sans-serif';
        ctx.fillStyle = '#9ca3af';
        ctx.textAlign = 'center';
        ctx.fillText(node.role, node.x, node.y + size + 6);
    }, []);

    // Click handler for God Mode (Architecture §10.2)
    const handleNodeClick = useCallback((node) => {
        setSelectedAgent(node);
        // Center the view on clicked node
        if (graphRef.current) {
            graphRef.current.centerAt(node.x, node.y, 500);
            graphRef.current.zoom(3, 500);
        }
    }, [setSelectedAgent]);

    // Auto-fit on graph change
    useEffect(() => {
        if (graphRef.current && graphData.nodes.length > 0) {
            setTimeout(() => {
                graphRef.current.zoomToFit(400, 60);
            }, 300);
        }
    }, [graphData.nodes.length]);

    return (
        <div className="relative w-full h-full">
            <ForceGraph2D
                ref={graphRef}
                graphData={graphData}
                nodeCanvasObject={paintNode}
                nodePointerAreaPaint={(node, color, ctx) => {
                    const size = Math.max(4, Math.min(16, (node.budget || 1) * 2));
                    ctx.beginPath();
                    ctx.arc(node.x, node.y, size + 4, 0, 2 * Math.PI);
                    ctx.fillStyle = color;
                    ctx.fill();
                }}
                linkDirectionalArrowLength={6}
                linkDirectionalArrowRelPos={1}
                linkDirectionalParticles={2}
                linkDirectionalParticleSpeed={0.005}
                linkDirectionalParticleWidth={2}
                linkColor={() => '#374151'}
                linkWidth={1.5}
                backgroundColor="#0a0e17"
                onNodeClick={handleNodeClick}
                cooldownTicks={100}
                d3AlphaDecay={0.02}
                d3VelocityDecay={0.3}
            />
            {/* Legend */}
            <div className="absolute top-4 left-4 bg-gray-900/80 backdrop-blur-sm rounded-lg p-3 border border-gray-700/50 text-xs">
                <div className="text-gray-400 font-semibold mb-2">RÔLES</div>
                <div className="flex flex-col gap-1.5">
                    <div className="flex items-center gap-2">
                        <span className="w-3 h-3 rounded-full bg-blue-500 inline-block" />
                        <span className="text-gray-300">Architecte</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <span className="w-3 h-3 rounded-full bg-emerald-500 inline-block" />
                        <span className="text-gray-300">Worker</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <span className="w-3 h-3 rounded-full bg-red-500 inline-block" />
                        <span className="text-gray-300">Critique</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <span className="w-3 h-3 rounded-full bg-violet-500 inline-block" />
                        <span className="text-gray-300">Codeur</span>
                    </div>
                </div>
            </div>
        </div>
    );
}
