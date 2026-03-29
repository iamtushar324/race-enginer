import { useState, useCallback } from 'react';
import { useWebSocket } from './hooks/useWebSocket';
import { useSettings } from './hooks/useSettings';
import { useVoice } from './hooks/useVoice';
import { usePushToTalk } from './hooks/usePushToTalk';
import { Header } from './components/Header';
import { SettingsModal } from './components/SettingsModal';
import { LiveTelemetry } from './components/panels/LiveTelemetry';
import { TireDamage } from './components/panels/TireDamage';
import { WeatherSession } from './components/panels/WeatherSession';
import { CommsControl } from './components/panels/CommsControl';
import { MockControlPanel } from './components/panels/MockControlPanel';
import { AgentPanel } from './components/panels/AgentPanel';

function App() {
  const { state, connected, health, log, addEntry, isSpeaking, pttActive } = useWebSocket();
  const { settings, saving, saveMode, saveTalkLevel, saveVerbosity, saveMockOverrides } = useSettings();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const handleTranscription = useCallback((text: string) => {
    addEntry(`[DRIVER] ${text}`, 'driver');
  }, [addEntry]);

  const voice = useVoice({ onTranscription: handleTranscription });

  usePushToTalk(voice.startRecording, voice.stopRecording, !settingsOpen, pttActive);

  const handleDriverMessage = useCallback((message: string) => {
    addEntry(`[DRIVER] ${message}`, 'driver');
  }, [addEntry]);

  const handleTalkLevel = useCallback((level: number) => {
    saveTalkLevel(level);
  }, [saveTalkLevel]);

  const handleVerbosity = useCallback((level: number) => {
    saveVerbosity(level);
  }, [saveVerbosity]);

  // Null state -- waiting for first data
  if (!state) {
    return (
      <div className="h-screen flex flex-col p-3">
        <Header
          health={health}
          connected={connected}
          onSettingsClick={() => setSettingsOpen(true)}
        />
        <SettingsModal
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          settings={settings}
          saving={saving}
          onSave={saveMode}
        />
        <div className="flex-1 flex items-center justify-center">
          <div className="text-center">
            <div className="text-2xl font-bold text-muted mb-2">Waiting for telemetry data...</div>
            <div className="text-sm text-[#555]">
              Start the Go telemetry service and ensure F1 25 is sending data
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col p-3">
      <Header
        health={health}
        connected={connected}
        onSettingsClick={() => setSettingsOpen(true)}
      />
      <SettingsModal
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings}
        saving={saving}
        onSave={saveMode}
      />
      <div className="grid grid-cols-[1.2fr_1fr_1fr_1fr_1.1fr] gap-3 flex-grow overflow-hidden">
        <LiveTelemetry data={state} />
        <TireDamage data={state} />
        <WeatherSession data={state} />
        <MockControlPanel
          enabled={settings?.mock_mode ?? false}
          overrides={settings?.mock_overrides}
          onSave={saveMockOverrides}
        />
        <CommsControl
          log={log}
          talkLevel={settings?.talk_level ?? 5}
          verbosity={settings?.verbosity ?? 5}
          onTalkLevelChange={handleTalkLevel}
          onVerbosityChange={handleVerbosity}
          onDriverMessage={handleDriverMessage}
          isSpeaking={isSpeaking}
          isRecording={voice.isRecording}
          isTranscribing={voice.isTranscribing}
          audioLevel={voice.audioLevel}
          voiceError={voice.error}
          onStartRecording={voice.startRecording}
          onStopRecording={voice.stopRecording}
        />
      </div>
      <div className="mt-3 shrink-0">
        <AgentPanel />
      </div>
    </div>
  );
}

export default App;
