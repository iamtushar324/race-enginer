/** Mirrors Go DrivingInsight struct */
export interface DrivingInsight {
  message: string;
  type: 'warning' | 'info' | 'encouragement' | 'strategy';
  priority: number;
}

export interface LogEntry {
  time: string;
  message: string;
  type: DrivingInsight['type'] | 'system' | 'driver';
}
