import { useState } from 'react';
import { useAgentStatus } from '../../hooks/useAgentStatus';

const levelColor: Record<string, string> = {
  info: '#58a6ff',
  warn: '#d29922',
  error: '#f85149',
  debug: '#666',
};

export function AgentPanel() {
  const { status, error } = useAgentStatus();
  const [open, setOpen] = useState(false);

  if (!status) return null;

  const dot = status.enabled
    ? status.healthy
      ? '#2ea043'
      : '#d29922'
    : '#555';

  const label = status.enabled
    ? status.healthy
      ? 'Connected'
      : 'Waiting…'
    : 'Disabled';

  return (
    <div className="bg-panel border border-border rounded-lg overflow-hidden">
      {/* Header bar — always visible, click to toggle */}
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-3.5 py-2.5 cursor-pointer hover:bg-[#1a1a1a] transition-colors border-none bg-transparent text-left"
      >
        <div className="flex items-center gap-2">
          <span
            className="inline-block w-2 h-2 rounded-full"
            style={{ backgroundColor: dot }}
          />
          <span className="text-[13px] text-muted uppercase tracking-wider font-medium">
            OpenCode Analyst
          </span>
          <span className="text-[11px] font-mono" style={{ color: dot }}>
            {label}
          </span>
        </div>
        <div className="flex items-center gap-3">
          {status.enabled && status.session_id && (
            <span className="text-[11px] text-[#555] font-mono" title={status.session_id}>
              sid:{status.session_id.slice(0, 8)}
            </span>
          )}
          {status.enabled && (
            <span className="text-[11px] text-[#555] font-mono">
              {status.cycles} cycles
            </span>
          )}
          <span className="text-[11px] text-[#555]">{open ? '\u25B2' : '\u25BC'}</span>
        </div>
      </button>

      {/* Collapsible body */}
      {open && (
        <div className="border-t border-border px-3.5 py-3">
          {error && (
            <div className="text-[11px] text-[#f85149] bg-[#1a0000] border border-[#da3633] rounded px-2 py-1 mb-2">
              {error}
            </div>
          )}

          {!status.enabled ? (
            <div className="text-[12px] text-[#555] italic">
              Agent mode is set to &quot;internal&quot;. Set ANALYST_MODE=opencode to enable the OpenCode agent.
            </div>
          ) : (
            <>
              {/* Status chips */}
              <div className="flex flex-wrap gap-2 mb-3">
                <Chip label="URL" value={status.url} />
                <Chip label="Session" value={status.session_id || '—'} />
                <Chip label="Interval" value={`${status.interval_sec}s`} />
                {status.last_cycle && (
                  <Chip
                    label="Last"
                    value={new Date(status.last_cycle).toLocaleTimeString()}
                  />
                )}
              </div>

              {/* Log entries */}
              <div className="text-[11px] text-muted uppercase tracking-wider mb-1.5">
                Agent Logs
              </div>
              <ul
                className="list-none overflow-y-auto font-mono text-xs"
                style={{ maxHeight: 280 }}
              >
                {status.logs.length === 0 ? (
                  <li className="text-[#555] italic px-1 py-1">No logs yet</li>
                ) : (
                  status.logs.map((entry, i) => (
                    <li
                      key={i}
                      className="px-1.5 py-1 border-b border-[#1a1a1a]"
                    >
                      <span className="text-[#555] mr-1.5">{entry.time}</span>
                      <span
                        className="mr-1.5 uppercase text-[9px] font-bold"
                        style={{ color: levelColor[entry.level] || '#888' }}
                      >
                        {entry.level}
                      </span>
                      <span style={{ color: levelColor[entry.level] || '#c9d1d9' }}>
                        {entry.message}
                      </span>
                    </li>
                  ))
                )}
              </ul>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function Chip({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-[#161b22] border border-[#30363d] rounded px-2 py-0.5 text-[11px]">
      <span className="text-[#555] mr-1">{label}:</span>
      <span className="text-[#c9d1d9] font-mono">{value}</span>
    </div>
  );
}
