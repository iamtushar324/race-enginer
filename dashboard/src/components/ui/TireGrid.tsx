import { TIRE_LABELS } from '../../lib/constants';

interface TireGridProps {
  /** Values in display order: [FL, FR, RL, RR] (already reordered) */
  values: number[];
  /** Format function for display */
  format?: (v: number) => string;
  /** Returns color based on value */
  colorFn: (v: number) => string;
  unit?: string;
}

export function TireGrid({ values, format, colorFn, unit }: TireGridProps) {
  const fmt = format ?? ((v: number) => v.toString());
  return (
    <div className="grid grid-cols-2 gap-1.5">
      {TIRE_LABELS.map((label, i) => (
        <div key={label} className="bg-black p-2 rounded-md text-center border border-[#222]">
          <div className="text-muted text-[10px] uppercase">{label}</div>
          <div className="text-base font-bold mt-0.5" style={{ color: colorFn(values[i]) }}>
            {fmt(values[i])}
            {unit && <span className="text-[10px] ml-0.5 text-[#888]">{unit}</span>}
          </div>
        </div>
      ))}
    </div>
  );
}
