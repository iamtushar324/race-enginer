import { useState, useEffect, useRef, useCallback } from 'react';
import type { RaceState } from '../types/telemetry';
import type { DrivingInsight, LogEntry } from '../types/insights';
import type { HealthStatus } from '../types/settings';
import { timestamp } from '../lib/format';

const MAX_LOG = 50;

function getWsUrl(): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}/ws`;
}

/**
 * Shared AudioContext that gets unlocked on first user interaction.
 * Browsers require a user gesture before allowing audio playback.
 */
let sharedAudioCtx: AudioContext | null = null;
let audioUnlocked = false;

function getAudioContext(): AudioContext {
  if (!sharedAudioCtx) {
    sharedAudioCtx = new AudioContext();
  }
  return sharedAudioCtx;
}

function unlockAudio() {
  if (audioUnlocked) return;
  const ctx = getAudioContext();
  if (ctx.state === 'suspended') {
    ctx.resume();
  }
  // Play a tiny silent buffer to fully unlock playback.
  const buf = ctx.createBuffer(1, 1, 22050);
  const src = ctx.createBufferSource();
  src.buffer = buf;
  src.connect(ctx.destination);
  src.start();
  audioUnlocked = true;
}

// Unlock audio on first user interaction (click, keydown, touchstart).
if (typeof window !== 'undefined') {
  const events = ['click', 'keydown', 'touchstart'] as const;
  const handler = () => {
    unlockAudio();
    events.forEach(e => window.removeEventListener(e, handler));
  };
  events.forEach(e => window.addEventListener(e, handler, { once: false }));
}

export function useWebSocket() {
  const [state, setState] = useState<RaceState | null>(null);
  const [connected, setConnected] = useState(false);
  const [health, setHealth] = useState<HealthStatus | null>(null);
  const [isSpeaking, setIsSpeaking] = useState(false);
  const [pttActive, setPttActive] = useState(false);
  const [log, setLog] = useState<LogEntry[]>([
    { time: timestamp(), message: 'System Ready.', type: 'info' },
  ]);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const backoff = useRef(1000);
  const audioQueue = useRef<string[]>([]);
  const isPlayingRef = useRef(false);
  const onInsightRef = useRef<((insight: DrivingInsight) => void) | undefined>();

  const addEntry = useCallback((message: string, type: LogEntry['type']) => {
    setLog(prev => [
      { time: timestamp(), message, type },
      ...prev,
    ].slice(0, MAX_LOG));
  }, []);

  const setOnInsight = useCallback((cb: (insight: DrivingInsight) => void) => {
    onInsightRef.current = cb;
  }, []);

  // Audio playback — decodes base64 MP3 via AudioContext for reliable playback.
  const playNext = useCallback(() => {
    if (audioQueue.current.length === 0) {
      isPlayingRef.current = false;
      setIsSpeaking(false);
      return;
    }
    isPlayingRef.current = true;
    setIsSpeaking(true);

    const base64 = audioQueue.current.shift()!;
    const raw = atob(base64);
    const buf = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i++) buf[i] = raw.charCodeAt(i);

    const ctx = getAudioContext();
    // Ensure context is running (may be suspended if user hasn't interacted yet).
    if (ctx.state === 'suspended') ctx.resume();

    ctx.decodeAudioData(
      buf.buffer,
      (audioBuffer) => {
        const source = ctx.createBufferSource();
        source.buffer = audioBuffer;
        source.connect(ctx.destination);
        source.onended = () => playNext();
        source.start();
      },
      () => {
        // Decode failed — try next.
        console.warn('[useWebSocket] Failed to decode audio, skipping');
        playNext();
      },
    );
  }, []);

  const enqueueAudio = useCallback((base64: string) => {
    audioQueue.current.push(base64);
    if (!isPlayingRef.current) {
      playNext();
    }
  }, [playNext]);

  useEffect(() => {
    let unmounted = false;

    function connect() {
      if (unmounted) return;

      const ws = new WebSocket(getWsUrl());
      wsRef.current = ws;

      ws.onopen = () => {
        if (unmounted) return;
        setConnected(true);
        backoff.current = 1000;
      };

      ws.onclose = () => {
        if (unmounted) return;
        setConnected(false);
        wsRef.current = null;
        // Exponential backoff reconnect: 1s -> 2s -> 4s -> max 10s.
        reconnectTimer.current = setTimeout(() => {
          backoff.current = Math.min(backoff.current * 2, 10000);
          connect();
        }, backoff.current);
      };

      ws.onerror = () => {
        // onclose will fire after onerror, triggering reconnect.
        ws.close();
      };

      ws.onmessage = (event) => {
        if (unmounted) return;
        try {
          const msg = JSON.parse(event.data) as { type: string; data: unknown };
          switch (msg.type) {
            case 'telemetry':
              setState(msg.data as RaceState);
              break;
            case 'health':
              setHealth(msg.data as HealthStatus);
              break;
            case 'insight': {
              const insight = msg.data as DrivingInsight;
              const logMsg = `[ENGINEER] ${insight.message}`;
              setLog(prev => [
                { time: timestamp(), message: logMsg, type: insight.type },
                ...prev,
              ].slice(0, MAX_LOG));
              onInsightRef.current?.(insight);
              break;
            }
            case 'audio': {
              const audioMsg = msg as { type: string; data: string; format: string };
              enqueueAudio(audioMsg.data);
              break;
            }
            case 'ptt': {
              const pttMsg = msg.data as { active: boolean };
              setPttActive(pttMsg.active);
              break;
            }
          }
        } catch {
          // Ignore malformed messages.
        }
      };
    }

    connect();

    return () => {
      unmounted = true;
      clearTimeout(reconnectTimer.current);
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [enqueueAudio]);

  return { state, connected, health, log, addEntry, isSpeaking, pttActive, setOnInsight };
}
