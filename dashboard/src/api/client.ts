import { API_BASE } from '../lib/constants';
import type { RaceState } from '../types/telemetry';
import type { DrivingInsight } from '../types/insights';
import type { HealthStatus, Settings, TelemetryModePayload, TalkLevelPayload, VerbosityPayload, MockOverrides } from '../types/settings';

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export async function fetchTelemetry(): Promise<RaceState | null> {
  const res = await fetch(`${API_BASE}/api/telemetry/latest`);
  if (res.status === 503) return null;
  if (!res.ok) throw new Error(`${res.status}`);
  return res.json();
}

export async function fetchNextInsight(): Promise<DrivingInsight | null> {
  const res = await fetch(`${API_BASE}/api/insights/next`);
  if (res.status === 204) return null;
  if (!res.ok) throw new Error(`${res.status}`);
  return res.json();
}

export async function fetchHealth(): Promise<HealthStatus> {
  return get('/health');
}

export async function fetchSettings(): Promise<Settings> {
  return get('/api/settings');
}

export async function postMode(payload: TelemetryModePayload) {
  return post('/api/settings/mode', payload);
}

export async function postTalkLevel(payload: TalkLevelPayload) {
  return post('/api/settings/talk_level', payload);
}

export async function postVerbosity(payload: VerbosityPayload) {
  return post('/api/settings/verbosity', payload);
}

export async function postMockOverrides(payload: MockOverrides) {
  return post('/api/settings/mock/overrides', payload);
}

// --- OpenCode Agent ---

export interface AgentLogEntry {
  time: string;
  level: string;
  message: string;
}

export interface AgentStatusResponse {
  enabled: boolean;
  healthy: boolean;
  url: string;
  session_id: string;
  interval_sec: number;
  cycles: number;
  last_cycle: string;
  logs: AgentLogEntry[];
}

export async function fetchAgentStatus(): Promise<AgentStatusResponse> {
  return get('/api/agent/status');
}
