import type { LogEntry } from '../../types/insights';

const typeColor: Record<string, string> = {
  info: '#58a6ff',
  warning: '#d29922',
  encouragement: '#2ea043',
  strategy: '#bc8cff',
  system: '#58a6ff',
  driver: '#58a6ff',
  event: '#FF9300',
  agent: '#a5d6ff',
};

interface LogListProps {
  entries: LogEntry[];
  maxHeight?: string;
}

export function LogList({ entries, maxHeight = '100%' }: LogListProps) {
  return (
    <ul
      className="list-none flex-grow overflow-y-auto font-mono text-xs"
      style={{ maxHeight }}
    >
      {entries.map((entry, i) => (
        <li key={i} className="px-1.5 py-1.5 border-b border-[#1a1a1a]">
          <span className="text-[#555] mr-1.5">{entry.time}</span>
          <span style={{ color: typeColor[entry.type] || '#c9d1d9' }}>{entry.message}</span>
        </li>
      ))}
    </ul>
  );
}
