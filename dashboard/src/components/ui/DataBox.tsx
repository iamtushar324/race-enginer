interface DataBoxProps {
  label: string;
  value: string | number;
  unit?: string;
  size?: 'lg' | 'sm' | 'xs';
  children?: React.ReactNode;
}

const sizeClass = { lg: 'text-[26px]', sm: 'text-lg', xs: 'text-sm' };

export function DataBox({ label, value, unit, size = 'lg', children }: DataBoxProps) {
  return (
    <div className="bg-black p-2.5 rounded-md text-center border border-[#222]">
      <div className="text-muted text-[10px] uppercase">{label}</div>
      <div className={`font-bold text-white mt-0.5 tabular-nums ${sizeClass[size]}`}>
        {value}
      </div>
      {unit && <div className="text-[#555] text-[10px]">{unit}</div>}
      {children}
    </div>
  );
}
