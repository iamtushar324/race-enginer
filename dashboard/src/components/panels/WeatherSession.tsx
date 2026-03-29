import { useEffect, useRef, useState } from 'react';
import type { RaceState } from '../../types/telemetry';
import { Indicator } from '../ui/Indicator';
import { LogList } from '../ui/LogList';
import type { LogEntry } from '../../types/insights';
import { formatLapTime, formatTimeLeft, formatDelta, timestamp } from '../../lib/format';
import {
  WEATHER_ICONS, WEATHER_NAMES, SAFETY_CAR_NAMES, EVENT_NAMES,
  BUTTON_FLAGS, decodeButtons,
} from '../../lib/constants';

interface Props {
  data: RaceState;
}

export function WeatherSession({ data }: Props) {
  const [events, setEvents] = useState<LogEntry[]>([]);
  const [bestLap, setBestLap] = useState(0);
  const [showButtonMap, setShowButtonMap] = useState(false);
  const prevEventRef = useRef('');
  const prevButtonRef = useRef(0);
  const prevLastLapRef = useRef(0);

  // Track race events
  useEffect(() => {
    if (data.last_event_code && data.last_event_code !== prevEventRef.current) {
      prevEventRef.current = data.last_event_code;

      // For BUTN events, decode the button bitmask into names
      if (data.last_event_code === 'BUTN' && data.last_button_status) {
        if (data.last_button_status !== prevButtonRef.current) {
          prevButtonRef.current = data.last_button_status;
          const names = decodeButtons(data.last_button_status);
          const hex = '0x' + data.last_button_status.toString(16).toUpperCase().padStart(8, '0');
          const msg = names.length > 0
            ? `Button: ${names.join(' + ')} (${hex})`
            : `Button: ${hex}`;
          setEvents(prev => [
            { time: timestamp(), message: msg, type: 'info' as const },
            ...prev,
          ].slice(0, 50));
        }
        return;
      }

      const name = EVENT_NAMES[data.last_event_code] ?? data.last_event_code;
      setEvents(prev => [
        { time: timestamp(), message: name, type: 'info' as const },
        ...prev,
      ].slice(0, 50));
    }
  }, [data.last_event_code, data.last_button_status]);

  // Track best lap
  useEffect(() => {
    if (data.last_lap_time_ms > 0 && data.last_lap_time_ms !== prevLastLapRef.current) {
      prevLastLapRef.current = data.last_lap_time_ms;
      if (bestLap === 0 || data.last_lap_time_ms < bestLap) {
        setBestLap(data.last_lap_time_ms);
      }
    }
  }, [data.last_lap_time_ms, bestLap]);

  const weatherIcon = WEATHER_ICONS[data.weather] ?? '?';
  const weatherName = WEATHER_NAMES[data.weather] ?? '?';
  const scStatus = data.safety_car_status;
  const scName = SAFETY_CAR_NAMES[scStatus] ?? '';

  return (
    <div className="bg-panel border border-border rounded-lg p-3.5 flex flex-col overflow-y-auto">
      <h2 className="text-[13px] text-muted uppercase tracking-wider mb-2.5">Weather & Session</h2>

      {/* Weather */}
      <div className="bg-black p-2.5 rounded-md border border-[#222] mb-2.5">
        <div className="flex items-center justify-center">
          <span className="text-[28px] mr-2">{weatherIcon}</span>
          <span className="text-lg font-bold">{weatherName}</span>
        </div>
        <div className="mt-1.5 flex justify-center gap-4">
          <span>
            <span className="text-muted text-[10px] uppercase">Track </span>
            <span className="font-bold">{data.track_temperature}</span>
            <span className="text-[10px]">C</span>
          </span>
          <span>
            <span className="text-muted text-[10px] uppercase">Air </span>
            <span className="font-bold">{data.air_temperature}</span>
            <span className="text-[10px]">C</span>
          </span>
        </div>
      </div>

      {/* Safety Car */}
      {scStatus > 0 && (
        <div className="mb-2 text-center">
          <Indicator
            text={scName}
            variant={scStatus === 1 ? 'yellow' : 'blue'}
          />
        </div>
      )}

      {/* Session Info */}
      <h3 className="text-xs text-muted uppercase tracking-tight mt-1 mb-1.5">Session Info</h3>
      <div className="space-y-0">
        <InfoRow label="Time Left" value={formatTimeLeft(data.session_time_left)} />
        <InfoRow label="Position" value={`P${data.position}`} />
        <InfoRow label="Gap to Front" value={data.delta_to_front_ms ? formatDelta(data.delta_to_front_ms) : '--'} />
        <InfoRow label="Gap to Leader" value={data.delta_to_leader_ms ? formatDelta(data.delta_to_leader_ms) : 'Leader'} />
        <InfoRow label="Pit Window" value={`Lap ${data.pit_window_ideal_lap}-${data.pit_window_latest_lap}`} />
        <InfoRow label="Pit Stops" value={data.num_pit_stops.toString()} />
      </div>

      {/* Lap Times */}
      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Lap Times</h3>
      <div className="space-y-0">
        <InfoRow label="Last Lap" value={formatLapTime(data.last_lap_time_ms)} mono />
        <InfoRow label="Best Lap" value={formatLapTime(bestLap)} mono purple />
      </div>

      {/* Race Events */}
      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Race Events</h3>
      <LogList entries={events} maxHeight="120px" />

      {/* Button Map Reference */}
      <button
        onClick={() => setShowButtonMap(v => !v)}
        className="mt-2 text-[10px] text-muted hover:text-white cursor-pointer bg-transparent border-none p-0 underline text-left"
      >
        {showButtonMap ? 'Hide' : 'Show'} Button Map
      </button>
      {showButtonMap && (
        <div className="mt-1 bg-black rounded border border-[#222] p-2 max-h-[200px] overflow-y-auto">
          <table className="w-full text-[10px] font-mono">
            <thead>
              <tr className="text-muted">
                <th className="text-left pr-2 pb-1">Hex</th>
                <th className="text-left pb-1">Button</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(BUTTON_FLAGS).map(([mask, name]) => (
                <tr key={mask} className="border-t border-[#1a1a1a]">
                  <td className="pr-2 py-0.5 text-[#58a6ff]">
                    0x{Number(mask).toString(16).toUpperCase().padStart(8, '0')}
                  </td>
                  <td className="py-0.5">{name}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function InfoRow({ label, value, mono, purple }: {
  label: string; value: string; mono?: boolean; purple?: boolean;
}) {
  return (
    <div className="flex justify-between py-1 border-b border-[#1a1a1a]">
      <span className="text-muted text-xs">{label}</span>
      <span
        className={`text-[13px] font-bold ${mono ? 'font-mono' : ''}`}
        style={purple ? { color: '#bc8cff' } : undefined}
      >
        {value}
      </span>
    </div>
  );
}
