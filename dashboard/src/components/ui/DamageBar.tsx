import { DAMAGE_THRESHOLDS } from '../../lib/constants';

interface DamageBarProps {
  label: string;
  value: number; // 0-100 percentage
}

function dmgColor(v: number): string {
  if (v < DAMAGE_THRESHOLDS.good) return '#2ea043';
  if (v < DAMAGE_THRESHOLDS.warn) return '#d29922';
  return '#f85149';
}

export function DamageBar({ label, value }: DamageBarProps) {
  const color = dmgColor(value);
  return (
    <div className="flex items-center gap-2 my-0.5">
      <span className="text-[11px] text-muted min-w-[70px]">{label}</span>
      <div className="flex-1 bg-[#222] h-1.5 rounded-full overflow-hidden">
        <div
          className="h-full transition-[width] duration-300"
          style={{ width: `${value}%`, background: color }}
        />
      </div>
      <span className="text-[11px] min-w-[30px] text-right" style={{ color }}>{value}%</span>
    </div>
  );
}
