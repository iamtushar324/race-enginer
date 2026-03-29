/** GET /api/settings response */
export interface Settings {
  mock_mode: boolean;
  talk_level: number;
  verbosity: number;
  udp_host: string;
  udp_port: number;
  udp_mode: string;
  api_port: number;
  db_path: string;
  python_api: string;
  mock_overrides?: MockOverrides;
}

export interface MockOverrides {
  tire_wear_multiplier: number;
  fuel_burn_multiplier: number;
  tire_temp_offset: number;
  weather_override: number | null;
  rain_percentage: number | null;
}

/** POST /api/settings/mode body */
export interface TelemetryModePayload {
  mode: 'mock' | 'real';
  host?: string;
  port?: number;
  udp_mode?: 'broadcast' | 'unicast';
}

/** POST /api/settings/talk_level body */
export interface TalkLevelPayload {
  talk_level: number;
}

/** POST /api/settings/verbosity body */
export interface VerbosityPayload {
  verbosity: number;
}

/** GET /health response */
export interface HealthStatus {
  status: string;
  uptime: string;
  packets_rx: number;
  duckdb_ok: boolean;
  mock_mode: boolean;
  talk_level: number;
  udp_host: string;
  udp_port: number;
  udp_mode: string;
}
