import { useRef, useEffect } from 'react';
import useStore from '../../store/useStore';

const LEVEL_COLORS = {
    info: 'text-gray-300',
    system: 'text-blue-400',
    warning: 'text-amber-400',
    error: 'text-red-400',
};

/**
 * Console - Live log stream from the backend WebSocket.
 * Per Architecture §10 (Console component).
 */
export default function Console() {
    const logs = useStore((s) => s.logs);
    const endRef = useRef(null);

    // Auto-scroll to bottom on new logs
    useEffect(() => {
        endRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [logs.length]);

    return (
        <div className="flex flex-col h-full">
            <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider px-4 py-2 border-b border-gray-700/30">
                Console — Live Stream
            </div>
            <div className="flex-1 overflow-y-auto px-4 py-2 font-mono text-xs space-y-0.5">
                {logs.length === 0 && (
                    <div className="text-gray-600 italic py-4">En attente de messages...</div>
                )}
                {logs.map((log, i) => (
                    <div key={i} className="flex gap-2 leading-5">
                        <span className="text-gray-600 shrink-0">
                            {new Date(log.timestamp).toLocaleTimeString('fr-FR')}
                        </span>
                        <span className={LEVEL_COLORS[log.level] || 'text-gray-300'}>
                            {log.message}
                        </span>
                    </div>
                ))}
                <div ref={endRef} />
            </div>
        </div>
    );
}
