import { useState, useEffect, useCallback } from 'react';
import { fetchAgentStatus, type AgentStatusResponse } from '../api/client';

const POLL_INTERVAL = 5000; // 5 seconds

export function useAgentStatus() {
  const [status, setStatus] = useState<AgentStatusResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const data = await fetchAgentStatus();
      setStatus(data);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch agent status');
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, POLL_INTERVAL);
    return () => clearInterval(id);
  }, [refresh]);

  return { status, error, refresh };
}
