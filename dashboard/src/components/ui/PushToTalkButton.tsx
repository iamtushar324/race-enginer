interface PushToTalkButtonProps {
  isRecording: boolean;
  isTranscribing: boolean;
  audioLevel: number; // 0-1
  onStartRecording: () => void;
  onStopRecording: () => void;
}

export function PushToTalkButton({
  isRecording,
  isTranscribing,
  audioLevel,
  onStartRecording,
  onStopRecording,
}: PushToTalkButtonProps) {
  const disabled = isTranscribing;

  const borderColor = isTranscribing
    ? '#d29922'
    : isRecording
      ? '#f85149'
      : '#58a6ff';

  const label = isTranscribing
    ? 'Transcribing...'
    : isRecording
      ? 'Release to Send'
      : 'Hold to Talk';

  const icon = isTranscribing
    ? null
    : isRecording
      ? null
      : '\uD83C\uDF99';

  return (
    <div className="relative w-full">
      <style>{`
        @keyframes ptt-pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.5; }
        }
      `}</style>
      <button
        disabled={disabled}
        onMouseDown={() => !disabled && onStartRecording()}
        onMouseUp={() => !disabled && onStopRecording()}
        onMouseLeave={() => isRecording && onStopRecording()}
        onTouchStart={() => !disabled && onStartRecording()}
        onTouchEnd={() => !disabled && onStopRecording()}
        className="w-full p-3 bg-black text-white rounded-md cursor-pointer font-bold text-[13px] transition-colors select-none disabled:opacity-50 disabled:cursor-not-allowed"
        style={{ border: `2px solid ${borderColor}` }}
      >
        <div className="flex items-center justify-center gap-2">
          {isRecording && (
            <span
              className="inline-block w-2.5 h-2.5 rounded-full bg-[#f85149]"
              style={{ animation: 'ptt-pulse 0.8s ease-in-out infinite' }}
            />
          )}
          {isTranscribing && (
            <span
              className="inline-block w-2.5 h-2.5 rounded-full bg-[#d29922]"
              style={{ animation: 'ptt-pulse 0.6s ease-in-out infinite' }}
            />
          )}
          {icon && <span>{icon}</span>}
          <span>{label}</span>
        </div>

        {/* Audio level bar */}
        {isRecording && (
          <div className="mt-2 h-1 w-full bg-[#1a1a1a] rounded-full overflow-hidden">
            <div
              className="h-full bg-[#f85149] rounded-full transition-all duration-75"
              style={{ width: `${Math.round(audioLevel * 100)}%` }}
            />
          </div>
        )}
      </button>
    </div>
  );
}
// TODO: Add game controller R2 support via Gamepad API
