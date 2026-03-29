import { useState, useEffect, useCallback } from 'react';
import type { Settings, TelemetryModePayload, TalkLevelPayload, VerbosityPayload, MockOverrides } from '../types/settings';
import { fetchSettings, postMode, postTalkLevel, postVerbosity, postMockOverrides } from '../api/client';

export function useSettings() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    fetchSettings().then(setSettings).catch(() => {});
  }, []);

  const saveMode = useCallback(async (payload: TelemetryModePayload) => {
    setSaving(true);
    try {
      await postMode(payload);
      const updated = await fetchSettings();
      setSettings(updated);
    } finally {
      setSaving(false);
    }
  }, []);

  const saveTalkLevel = useCallback(async (level: number) => {
    const payload: TalkLevelPayload = { talk_level: level };
    await postTalkLevel(payload);
    setSettings(prev => prev ? { ...prev, talk_level: level } : prev);
  }, []);

  const saveVerbosity = useCallback(async (level: number) => {
    const payload: VerbosityPayload = { verbosity: level };
    await postVerbosity(payload);
    setSettings(prev => prev ? { ...prev, verbosity: level } : prev);
  }, []);

  const saveMockOverrides = useCallback(async (overrides: MockOverrides) => {
    await postMockOverrides(overrides);
    setSettings(prev => prev ? { ...prev, mock_overrides: overrides } : prev);
  }, []);

  return { settings, saving, saveMode, saveTalkLevel, saveVerbosity, saveMockOverrides };
}
