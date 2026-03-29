import type { RaceState } from '../../types/telemetry';
import { DataBox } from '../ui/DataBox';
import { BarMeter } from '../ui/BarMeter';
import { TireGrid } from '../ui/TireGrid';
import { Indicator } from '../ui/Indicator';
import { formatGear } from '../../lib/format';
import {
  reorderTires, COMPOUND_NAMES, COMPOUND_COLORS,
  FUEL_MIX_NAMES, ERS_MODE_NAMES,
  ERS_MAX_JOULES, FUEL_TANK_MAX, RPM_MAX,
  TIRE_WEAR_THRESHOLDS,
} from '../../lib/constants';

interface Props {
  data: RaceState;
}

function wearColor(v: number): string {
  if (v < TIRE_WEAR_THRESHOLDS.good) return '#2ea043';
  if (v < TIRE_WEAR_THRESHOLDS.warn) return '#d29922';
  return '#f85149';
}

export function LiveTelemetry({ data }: Props) {
  const ersPct = (data.ers_store_energy / ERS_MAX_JOULES) * 100;
  const fuelPct = Math.min(100, (data.fuel_in_tank / FUEL_TANK_MAX) * 100);
  const rpmPct = Math.min(100, (data.engine_rpm / RPM_MAX) * 100);
  const wear = reorderTires(data.tyres_wear);
  const compName = COMPOUND_NAMES[data.visual_tyre_compound] ?? '?';
  const compColor = COMPOUND_COLORS[data.visual_tyre_compound] ?? '#fff';

  return (
    <div className="bg-panel border border-border rounded-lg p-3.5 flex flex-col overflow-y-auto">
      <h2 className="text-[13px] text-muted uppercase tracking-wider mb-2.5">Live Telemetry</h2>

      {/* Speed, Gear, Lap, RPM */}
      <div className="grid grid-cols-2 gap-2">
        <DataBox label="Speed" value={Math.round(data.speed)} unit="km/h" />
        <DataBox label="Gear" value={formatGear(data.gear)}>
          <div className="mt-1">
            <Indicator text="DRS" variant={data.drs ? 'green' : 'red'} hidden={!data.drs && !data.drs_allowed} />
          </div>
        </DataBox>
        <DataBox label="Lap" value={data.current_lap} unit={`Sector ${data.sector + 1}`} size="sm" />
        <DataBox label="RPM" value={Math.round(data.engine_rpm)} size="sm">
          <BarMeter value={rpmPct} color="#58a6ff" />
        </DataBox>
      </div>

      {/* Throttle / Brake */}
      <div className="mt-2.5 space-y-1.5">
        <BarMeter label="Throttle" value={data.throttle * 100} color="#2ea043" />
        <BarMeter label="Brake" value={data.brake * 100} color="#f85149" />
      </div>

      {/* ERS & Fuel */}
      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Energy & Fuel</h3>
      <div className="grid grid-cols-2 gap-2">
        <DataBox label="ERS" value={`${ersPct.toFixed(0)}%`} size="sm">
          <BarMeter value={ersPct} color="#FFD700" />
          <div className="text-[10px] text-[#555] mt-0.5">{ERS_MODE_NAMES[data.ers_deploy_mode] ?? '?'}</div>
        </DataBox>
        <DataBox label="Fuel" value={data.fuel_remaining_laps.toFixed(1)} size="sm">
          <BarMeter value={fuelPct} color="#FF9300" />
          <div className="text-[10px] text-[#555] mt-0.5">{FUEL_MIX_NAMES[data.fuel_mix] ?? '?'}</div>
        </DataBox>
      </div>

      {/* Compound */}
      <div className="mt-2 text-center">
        <Indicator text={compName} variant="red" color={compColor} />
        <span className="text-xs text-[#888] ml-1.5">Age: {data.tyres_age_laps} laps</span>
      </div>

      {/* Tire Wear */}
      <h3 className="text-xs text-muted uppercase tracking-tight mt-3 mb-1.5">Tire Wear</h3>
      <TireGrid
        values={wear}
        format={v => `${v.toFixed(1)}%`}
        colorFn={wearColor}
      />
    </div>
  );
}
