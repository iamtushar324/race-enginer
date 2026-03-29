import { useState, useEffect } from 'react';
import type { Settings, TelemetryModePayload } from '../types/settings';

interface Props {
  open: boolean;
  onClose: () => void;
  settings: Settings | null;
  saving: boolean;
  onSave: (payload: TelemetryModePayload) => void;
}

export function SettingsModal({ open, onClose, settings, saving, onSave }: Props) {
  const [mode, setMode] = useState<'mock' | 'real'>('mock');
  const [udpMode, setUdpMode] = useState<'broadcast' | 'unicast'>('broadcast');
  const [host, setHost] = useState('0.0.0.0');
  const [port, setPort] = useState('20777');
  const [showSaved, setShowSaved] = useState(false);

  // Sync form with settings
  useEffect(() => {
    if (settings) {
      setMode(settings.mock_mode ? 'mock' : 'real');
      setUdpMode(settings.udp_mode === 'unicast' ? 'unicast' : 'broadcast');
      setHost(settings.udp_host);
      setPort(settings.udp_port.toString());
    }
  }, [settings]);

  const handleSave = async () => {
    onSave({
      mode,
      host,
      port: parseInt(port) || 20777,
      udp_mode: udpMode,
    });
    setShowSaved(true);
    setTimeout(() => setShowSaved(false), 2000);
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 bg-black/60 z-[1000] flex justify-center items-center"
      onClick={e => e.target === e.currentTarget && onClose()}
    >
      <div className="bg-panel border border-border rounded-xl p-6 w-[400px] max-w-[90vw] shadow-[0_8px_32px_rgba(0,0,0,0.5)]">
        {/* Header */}
        <div className="flex justify-between items-center mb-5 pb-3 border-b border-border">
          <h2 className="text-base text-white font-bold">Settings</h2>
          <button
            onClick={onClose}
            className="bg-transparent border-none text-muted text-xl cursor-pointer p-0 leading-none hover:text-white"
          >
            &times;
          </button>
        </div>

        {/* Telemetry Mode */}
        <div className="mb-4">
          <label className="block text-xs text-muted uppercase tracking-wide mb-1.5">
            Telemetry Mode
          </label>
          <div className="flex border border-border rounded-md overflow-hidden">
            <ModeBtn label="Mock" active={mode === 'mock'} onClick={() => setMode('mock')} />
            <ModeBtn label="Real" active={mode === 'real'} onClick={() => setMode('real')} />
          </div>
        </div>

        {/* UDP Mode */}
        <div className="mb-4">
          <label className="block text-xs text-muted uppercase tracking-wide mb-1.5">
            UDP Stream Mode
          </label>
          <div className="flex border border-border rounded-md overflow-hidden">
            <ModeBtn label="Broadcast" active={udpMode === 'broadcast'} onClick={() => setUdpMode('broadcast')} />
            <ModeBtn label="Unicast" active={udpMode === 'unicast'} onClick={() => setUdpMode('unicast')} />
          </div>
          <div className="text-[11px] text-[#555] mt-1.5">
            {udpMode === 'broadcast'
              ? 'Game sends to all devices on the network'
              : "Game sends directly to this machine's IP"}
          </div>
        </div>

        {/* Host + Port */}
        <div className="flex gap-3 mb-4">
          <div className="flex-1">
            <label className="block text-xs text-muted uppercase tracking-wide mb-1.5">
              IP Address / Host
            </label>
            <input
              type="text"
              value={host}
              onChange={e => setHost(e.target.value)}
              placeholder="0.0.0.0"
              className="w-full px-3 py-2 bg-bg text-white border border-border rounded-md text-sm font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <div className="w-24">
            <label className="block text-xs text-muted uppercase tracking-wide mb-1.5">
              Port
            </label>
            <input
              type="number"
              value={port}
              onChange={e => setPort(e.target.value)}
              placeholder="20777"
              className="w-full px-3 py-2 bg-bg text-white border border-border rounded-md text-sm font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </div>

        {/* Save */}
        <button
          onClick={handleSave}
          disabled={saving}
          className="w-full py-2.5 bg-accent text-white border-none rounded-md cursor-pointer font-bold text-sm mt-2 hover:bg-[#4090e0] transition-colors disabled:opacity-50"
        >
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
        <div
          className={`text-center text-xs text-success mt-2 transition-opacity duration-300 ${showSaved ? 'opacity-100' : 'opacity-0'}`}
        >
          Settings saved successfully
        </div>
      </div>
    </div>
  );
}

function ModeBtn({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={`flex-1 py-2 px-4 text-center text-[13px] font-bold cursor-pointer border-none transition-all ${
        active
          ? 'bg-accent text-white'
          : 'bg-bg text-muted hover:bg-[#1a1f27] hover:text-white'
      }`}
    >
      {label}
    </button>
  );
}
