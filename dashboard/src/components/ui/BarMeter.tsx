interface BarMeterProps {
  label?: string;
  value: number; // 0-100
  color: string; // tailwind or hex color
}

export function BarMeter({ label, value, color }: BarMeterProps) {
  return (
    <div>
      {label && <div className="text-muted text-[10px] uppercase">{label}</div>}
      <div className="bg-[#222] h-2 rounded overflow-hidden mt-1">
        <div
          className="h-full transition-[width] duration-100"
          style={{ width: `${Math.min(100, Math.max(0, value))}%`, background: color }}
        />
      </div>
    </div>
  );
}
