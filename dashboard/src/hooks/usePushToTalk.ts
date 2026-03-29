import { useEffect, useRef } from 'react';

/**
 * Push-to-talk via spacebar OR F1 steering wheel button (via UDP BUTN events).
 * Ignores key events when focus is on INPUT, TEXTAREA, or SELECT.
 *
 * @param onDown  Called when PTT activates (spacebar press or F1 button press)
 * @param onUp    Called when PTT deactivates (spacebar release or F1 button release)
 * @param enabled Whether the hook is active
 * @param pttActive  Remote PTT state from WebSocket (F1 BUTN event)
 */
export function usePushToTalk(
  onDown: () => void,
  onUp: () => void,
  enabled: boolean = true,
  pttActive: boolean = false,
) {
  const prevPttRef = useRef(false);

  // Spacebar listener
  useEffect(() => {
    if (!enabled) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.code !== 'Space' || e.repeat) return;
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      e.preventDefault();
      onDown();
    };

    const handleKeyUp = (e: KeyboardEvent) => {
      if (e.code !== 'Space') return;
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      e.preventDefault();
      onUp();
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [onDown, onUp, enabled]);

  // F1 UDP BUTN listener (via WebSocket ptt messages)
  useEffect(() => {
    if (!enabled) return;

    const wasPtt = prevPttRef.current;
    prevPttRef.current = pttActive;

    if (pttActive && !wasPtt) {
      onDown();
    } else if (!pttActive && wasPtt) {
      onUp();
    }
  }, [pttActive, onDown, onUp, enabled]);
}
