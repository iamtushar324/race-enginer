import { useState, useRef, useCallback } from 'react';
import { API_BASE } from '../lib/constants';

interface UseVoiceOptions {
  onTranscription?: (text: string) => void;
  onError?: (msg: string) => void;
}

export function useVoice({ onTranscription, onError }: UseVoiceOptions = {}) {
  const [isRecording, setIsRecording] = useState(false);
  const [isTranscribing, setIsTranscribing] = useState(false);
  const [audioLevel, setAudioLevel] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const mediaRecorder = useRef<MediaRecorder | null>(null);
  const audioContext = useRef<AudioContext | null>(null);
  const analyser = useRef<AnalyserNode | null>(null);
  const animFrame = useRef<number>(0);
  const chunks = useRef<Blob[]>([]);

  const updateLevel = useCallback(() => {
    if (!analyser.current) return;
    const data = new Uint8Array(analyser.current.frequencyBinCount);
    analyser.current.getByteFrequencyData(data);
    const avg = data.reduce((sum, v) => sum + v, 0) / data.length;
    setAudioLevel(Math.min(avg / 128, 1));
    animFrame.current = requestAnimationFrame(updateLevel);
  }, []);

  const startRecording = useCallback(async () => {
    try {
      setError(null);
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });

      // Audio level analysis
      audioContext.current = new AudioContext();
      const source = audioContext.current.createMediaStreamSource(stream);
      analyser.current = audioContext.current.createAnalyser();
      analyser.current.fftSize = 256;
      source.connect(analyser.current);
      updateLevel();

      // MediaRecorder
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : MediaRecorder.isTypeSupported('audio/webm')
          ? 'audio/webm'
          : 'audio/mp4';

      chunks.current = [];
      const recorder = new MediaRecorder(stream, { mimeType });
      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) chunks.current.push(e.data);
      };
      recorder.onstop = async () => {
        // Cleanup stream
        stream.getTracks().forEach(t => t.stop());
        cancelAnimationFrame(animFrame.current);
        if (audioContext.current) {
          audioContext.current.close();
          audioContext.current = null;
        }
        setAudioLevel(0);

        const blob = new Blob(chunks.current, { type: mimeType });
        if (blob.size === 0) return;

        // Don't show "transcribing" state — the voice ack covers the wait.
        try {
          const form = new FormData();
          form.append('audio', blob, 'recording.webm');
          const resp = await fetch(`${API_BASE}/api/voice`, {
            method: 'POST',
            body: form,
          });
          if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
          const data = await resp.json();
          if (data.transcription) {
            onTranscription?.(data.transcription);
          }
        } catch (err) {
          const msg = err instanceof Error ? err.message : 'Transcription failed';
          setError(msg);
          onError?.(msg);
        }
      };

      mediaRecorder.current = recorder;
      recorder.start();
      setIsRecording(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Microphone access denied';
      setError(msg);
      onError?.(msg);
    }
  }, [onTranscription, onError, updateLevel]);

  const stopRecording = useCallback(() => {
    if (mediaRecorder.current && mediaRecorder.current.state !== 'inactive') {
      mediaRecorder.current.stop();
    }
    setIsRecording(false);
  }, []);

  return { isRecording, isTranscribing, audioLevel, error, startRecording, stopRecording };
}
