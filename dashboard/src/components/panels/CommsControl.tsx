import { useState, useRef } from 'react';
import { LogList } from '../ui/LogList';
import { VoiceStatusBar } from '../ui/VoiceStatusBar';
import { PushToTalkButton } from '../ui/PushToTalkButton';
import type { LogEntry } from '../../types/insights';
import { API_BASE } from '../../lib/constants';

interface Props {
  log: LogEntry[];
  talkLevel: number;
  verbosity: number;
  onTalkLevelChange: (level: number) => void;
  onVerbosityChange: (level: number) => void;
  onDriverMessage: (message: string) => void;
  isSpeaking: boolean;
  isRecording: boolean;
  isTranscribing: boolean;
  audioLevel: number;
  voiceError: string | null;
  onStartRecording: () => void;
  onStopRecording: () => void;
}

export function CommsControl({
  log,
  talkLevel,
  verbosity,
  onTalkLevelChange,
  onVerbosityChange,
  onDriverMessage,
  isSpeaking,
  isRecording,
  isTranscribing,
  audioLevel,
  voiceError,
  onStartRecording,
  onStopRecording,
}: Props) {
  const [input, setInput] = useState('');
  const [level, setLevel] = useState(talkLevel);
  const [verb, setVerb] = useState(verbosity);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleSend = () => {
    if (!input.trim()) return;
    onDriverMessage(input.trim());

    // Attempt to post to Go API
    fetch(`${API_BASE}/api/driver_query`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: input.trim() }),
    }).catch(() => {});

    setInput('');
    inputRef.current?.focus();
  };

  const handleLevelChange = (val: number) => {
    setLevel(val);
    onTalkLevelChange(val);
  };

  const handleVerbosityChange = (val: number) => {
    setVerb(val);
    onVerbosityChange(val);
  };

  return (
    <div className="bg-panel border border-border rounded-lg p-3.5 flex flex-col overflow-y-auto">
      <h2 className="text-[13px] text-muted uppercase tracking-wider mb-2.5">Race Engineer Comms</h2>

      {/* Voice Status */}
      <VoiceStatusBar
        isSpeaking={isSpeaking}
        isRecording={isRecording}
        isTranscribing={isTranscribing}
      />

      {/* Log */}
      <LogList entries={log} />

      {/* Voice Error */}
      {voiceError && (
        <div className="text-[11px] text-[#f85149] bg-[#1a0000] border border-[#da3633] rounded px-2 py-1 mt-2">
          {voiceError}
        </div>
      )}

      <div className="mt-2.5 pt-2.5 border-t border-border">
        {/* Push to Talk */}
        <div className="mb-3">
          <PushToTalkButton
            isRecording={isRecording}
            isTranscribing={isTranscribing}
            audioLevel={audioLevel}
            onStartRecording={onStartRecording}
            onStopRecording={onStopRecording}
          />
        </div>

        {/* Talk Level */}
        <div className="mb-2">
          <label className="text-xs font-bold">Talk Level:</label>
          <div className="flex items-center gap-1.5 mt-1">
            <span className="text-[10px] text-[#555]">Quiet</span>
            <input
              type="range"
              min={1}
              max={10}
              value={level}
              onChange={e => handleLevelChange(Number(e.target.value))}
              className="flex-grow accent-accent cursor-pointer"
            />
            <span className="text-[10px] text-[#555]">Chatty</span>
            <span className="w-5 text-center font-bold bg-[#222] px-1 py-0.5 rounded text-xs">
              {level}
            </span>
          </div>
        </div>

        {/* Verbosity */}
        <div className="mb-3">
          <label className="text-xs font-bold">Detail Level:</label>
          <div className="flex items-center gap-1.5 mt-1">
            <span className="text-[10px] text-[#555]">Concise</span>
            <input
              type="range"
              min={1}
              max={10}
              value={verb}
              onChange={e => handleVerbosityChange(Number(e.target.value))}
              className="flex-grow accent-[#bc8cff] cursor-pointer"
            />
            <span className="text-[10px] text-[#555]">Detailed</span>
            <span className="w-5 text-center font-bold bg-[#222] px-1 py-0.5 rounded text-xs">
              {verb}
            </span>
          </div>
        </div>

        {/* Driver Radio (text input) */}
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && handleSend()}
          placeholder="Driver radio message..."
          className="w-full p-2 bg-black text-white border border-border rounded-md mb-2 text-[13px] focus:outline-none focus:border-accent"
        />
        <button
          onClick={handleSend}
          className="w-full p-2 bg-border text-white border-none rounded-md cursor-pointer font-bold text-[13px] hover:bg-[#4b535d] transition-colors"
        >
          Send
        </button>
      </div>
    </div>
  );
}
