import { StatusChip } from './ui/StatusChip';
import type { HealthStatus } from '../types/settings';

interface Props {
  health: HealthStatus | null;
  connected: boolean;
  onSettingsClick: () => void;
}

export function Header({ health, connected, onSettingsClick }: Props) {
  const isMock = health?.mock_mode ?? true;
  const dotColor = connected
    ? '#2ea043'
    : '#f85149';
  const connText = isMock
    ? 'Mock Running'
    : connected
      ? 'Connected'
      : 'Waiting for game...';

  return (
    <div className="flex justify-between items-center border-b border-border pb-2 mb-3">
      <div className="flex items-center gap-2.5">
        <button
          onClick={onSettingsClick}
          title="Settings"
          className="bg-transparent border border-border rounded-md text-muted cursor-pointer px-2 py-1 text-base flex items-center hover:bg-panel hover:text-white hover:border-muted transition-all"
        >
          &#9881;
        </button>
        <h1 className="text-white text-xl font-bold">Race Engineer | Team Pit Wall</h1>
      </div>

      <div className="flex items-center gap-4">
        <StatusChip value={connText} dot={dotColor} />
        <StatusChip label="Mode" value={isMock ? 'MOCK' : 'REAL'} />
        <StatusChip label="Host" value={health?.udp_host ?? '0.0.0.0'} />
        <StatusChip label="Port" value={(health?.udp_port ?? 20777).toString()} />
        <StatusChip label="UDP" value={(health?.udp_mode ?? 'broadcast').toUpperCase()} />
      </div>
    </div>
  );
}
