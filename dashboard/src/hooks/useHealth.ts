import { useState, useEffect, useRef } from 'react';
import type { HealthStatus } from '../types/settings';
import { fetchHealth } from '../api/client';

export function useHealth(intervalMs = 5000) {
  const [health, setHealth] = useState<HealthStatus | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval>>();

  useEffect(() => {
    let active = true;
    const poll = async () => {
      try {
        const data = await fetchHealth();
        if (active) setHealth(data);
      } catch { /* ignore */ }
    };

    poll();
    timerRef.current = setInterval(poll, intervalMs);
    return () => {
      active = false;
      clearInterval(timerRef.current);
    };
  }, [intervalMs]);

  return health;
}
