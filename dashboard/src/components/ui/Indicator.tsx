type Variant = 'green' | 'red' | 'yellow' | 'blue' | 'purple';

const styles: Record<Variant, string> = {
  green: 'bg-[#1a3a1a] text-success border-success',
  red: 'bg-[#3a1a1a] text-danger border-danger',
  yellow: 'bg-[#3a3a1a] text-warning border-warning',
  blue: 'bg-[#1a1a3a] text-accent border-accent',
  purple: 'bg-[#2a1a3a] text-purple border-purple',
};

interface IndicatorProps {
  text: string;
  variant: Variant;
  hidden?: boolean;
  /** Override text color */
  color?: string;
}

export function Indicator({ text, variant, hidden, color }: IndicatorProps) {
  if (hidden) return null;
  return (
    <span
      className={`inline-block px-2 py-0.5 rounded text-[11px] font-bold border ${styles[variant]}`}
      style={color ? { color, borderColor: color } : undefined}
    >
      {text}
    </span>
  );
}
