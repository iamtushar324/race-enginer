interface StatusChipProps {
  label?: string;
  value: string;
  dot?: string; // color for status dot
}

export function StatusChip({ label, value, dot }: StatusChipProps) {
  return (
    <div className="flex items-center gap-1.5 bg-panel border border-border rounded-md px-2.5 py-1">
      {dot && <span className="w-2 h-2 rounded-full" style={{ background: dot }} />}
      {label && <span className="text-[10px] text-muted uppercase">{label}</span>}
      <span className="text-xs font-bold text-white font-mono">{value}</span>
    </div>
  );
}
