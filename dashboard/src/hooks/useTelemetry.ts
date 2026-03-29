import { useState, useEffect, useRef } from 'react';
import type { RaceState } from '../types/telemetry';
import { fetchTelemetry } from '../api/client';

export function useTelemetry(intervalMs = 200) {
  const [state, setState] = useState<RaceState | null>(null);
  const [connected, setConnected] = useState(false);
  const timerRef = useRef<ReturnType<typeof setInterval>>();

  useEffect(() => {
    let active = true;
    const poll = async () => {
      try {
        const data = await fetchTelemetry();
        if (!active) return;
        setState(data);
        setConnected(data !== null);
      } catch {
        if (active) setConnected(false);
      }
    };

    poll();
    timerRef.current = setInterval(poll, intervalMs);
    return () => {
      active = false;
      clearInterval(timerRef.current);
    };
  }, [intervalMs]);

  return { state, connected };
}
