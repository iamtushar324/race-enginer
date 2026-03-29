interface VoiceStatusBarProps {
  isSpeaking: boolean;
  isRecording: boolean;
  isTranscribing: boolean;
}

type VoiceState = 'standby' | 'recording' | 'transcribing' | 'speaking';

const stateConfig: Record<VoiceState, { label: string; hint: string; color: string; dotColor: string }> = {
  standby:      { label: 'STANDBY',         hint: 'Hold SPACE to talk', color: '#30363d', dotColor: '#555' },
  recording:    { label: 'RECORDING',        hint: '',                   color: '#da3633', dotColor: '#f85149' },
  transcribing: { label: 'TRANSCRIBING...', hint: '',                   color: '#9e6a03', dotColor: '#d29922' },
  speaking:     { label: 'ENGINEER SPEAKING', hint: '',                   color: '#238636', dotColor: '#2ea043' },
};

function resolveState(props: VoiceStatusBarProps): VoiceState {
  if (props.isRecording) return 'recording';
  if (props.isTranscribing) return 'transcribing';
  if (props.isSpeaking) return 'speaking';
  return 'standby';
}

export function VoiceStatusBar({ isSpeaking, isRecording, isTranscribing }: VoiceStatusBarProps) {
  const voiceState = resolveState({ isSpeaking, isRecording, isTranscribing });
  const config = stateConfig[voiceState];
  const isPulsing = voiceState !== 'standby';

  return (
    <>
      <style>{`
        @keyframes voice-pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.3; }
        }
      `}</style>
      <div
        className="flex items-center px-2.5 py-1.5 rounded-md mb-2 text-xs font-mono"
        style={{ backgroundColor: config.color, border: `1px solid ${config.dotColor}` }}
      >
        <span
          className="inline-block w-2 h-2 rounded-full mr-2 flex-shrink-0"
          style={{
            backgroundColor: config.dotColor,
            animation: isPulsing ? 'voice-pulse 1s ease-in-out infinite' : 'none',
          }}
        />
        <span className="font-bold tracking-wider">{config.label}</span>
        {config.hint && (
          <span className="ml-auto text-[#8b949e] text-[10px]">{config.hint}</span>
        )}
      </div>
    </>
  );
}
