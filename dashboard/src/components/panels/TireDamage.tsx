import type { RaceState } from '../../types/telemetry';
import { TireGrid } from '../ui/TireGrid';
import { DamageBar } from '../ui/DamageBar';
import {
  reorderTires,
  SURFACE_TEMP_THRESHOLDS,
  INNER_TEMP_THRESHOLDS,
  BRAKE_TEMP_THRESHOLDS,
} from '../../lib/constants';

interface Props {
  data: RaceState;
}

function tempColor(v: number, cold: number, hot: number): string {
  if (v < cold) return '#58a6ff';
  if (v > hot) return '#f85149';
  return '#2ea043';
}

export function TireDamage({ data }: Props) {
  const surfTemps = reorderTires(data.tyres_surface_temp);
  const innerTemps = reorderTires(data.tyres_inner_temp);
  const brakeTemps = reorderTires(data.brakes_temp);
  const pressures = reorderTires(data.tyres_pressure);

  return (
    <div className="bg-panel border border-border rounded-lg p-3.5 flex flex-col overflow-y-auto">
      <h2 className="text-[13px] text-muted uppercase tracking-wider mb-2.5">Tire & Damage</h2>

      <h3 className="text-xs text-muted uppercase tracking-tight mt-1 mb-1.5">Surface Temp (C)</h3>
      <TireGrid
        values={surfTemps}
        colorFn={v => tempColor(v, SURFACE_TEMP_THRESHOLDS.cold, SURFACE_TEMP_THRESHOLDS.hot)}
      />

      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Inner Temp (C)</h3>
      <TireGrid
        values={innerTemps}
        colorFn={v => tempColor(v, INNER_TEMP_THRESHOLDS.cold, INNER_TEMP_THRESHOLDS.hot)}
      />

      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Brake Temp (C)</h3>
      <TireGrid
        values={brakeTemps}
        colorFn={v => tempColor(v, BRAKE_TEMP_THRESHOLDS.cold, BRAKE_TEMP_THRESHOLDS.hot)}
      />

      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Tire Pressure (PSI)</h3>
      <TireGrid
        values={pressures}
        format={v => v.toFixed(1)}
        colorFn={() => '#c9d1d9'}
      />

      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Damage</h3>
      <div>
        <DamageBar label="FL Wing" value={data.front_left_wing_damage} />
        <DamageBar label="FR Wing" value={data.front_right_wing_damage} />
        <DamageBar label="Rear Wing" value={data.rear_wing_damage} />
        <DamageBar label="Floor" value={data.floor_damage} />
        <DamageBar label="Diffuser" value={data.diffuser_damage} />
        <DamageBar label="Sidepod" value={data.sidepod_damage} />
        <DamageBar label="Gearbox" value={data.gear_box_damage} />
        <DamageBar label="Engine" value={data.engine_damage} />
      </div>
    </div>
  );
}
