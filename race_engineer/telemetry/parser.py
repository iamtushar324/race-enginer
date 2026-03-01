import asyncio
import logging
import math
import random
import time
from typing import List

from race_engineer.core.event_bus import bus
from race_engineer.telemetry.enums import (
    EventCode,
    Weather,
    SafetyCarStatus,
    TyreCompound,
    TyreCompoundVisual,
    FuelMix,
    ERSDeployMode,
)
from race_engineer.telemetry.packets import (
    PacketHeader,
    CarMotionData,
    PacketMotionData,
    PacketSessionData,
    WeatherForecastSample,
    CarLapData,
    PacketLapData,
    PacketEventData,
    EventDataDetails,
    ParticipantData,
    PacketParticipantsData,
    CarSetup,
    PacketCarSetupData,
    CarTelemetry,
    PacketCarTelemetryData,
    CarStatus,
    PacketCarStatusData,
    CarDamage,
    PacketCarDamageData,
    LapHistoryData,
    TyreStintHistoryData,
    PacketSessionHistoryData,
    TyreSetData,
    PacketTyreSetsData,
    PacketMotionExData,
)

logger = logging.getLogger(__name__)

# AI driver grid for 20-car field
AI_DRIVERS = [
    {"name": "Player", "team_id": 8, "race_number": 4, "nationality": 10},
    {"name": "L. Norris", "team_id": 8, "race_number": 4, "nationality": 10},
    {"name": "M. Verstappen", "team_id": 2, "race_number": 1, "nationality": 22},
    {"name": "S. Perez", "team_id": 2, "race_number": 11, "nationality": 52},
    {"name": "L. Hamilton", "team_id": 1, "race_number": 44, "nationality": 10},
    {"name": "C. Leclerc", "team_id": 1, "race_number": 16, "nationality": 53},
    {"name": "G. Russell", "team_id": 0, "race_number": 63, "nationality": 10},
    {"name": "A. Antonelli", "team_id": 0, "race_number": 12, "nationality": 41},
    {"name": "F. Alonso", "team_id": 4, "race_number": 14, "nationality": 77},
    {"name": "L. Stroll", "team_id": 4, "race_number": 18, "nationality": 13},
    {"name": "P. Gasly", "team_id": 5, "race_number": 10, "nationality": 28},
    {"name": "J. Doohan", "team_id": 5, "race_number": 7, "nationality": 3},
    {"name": "Y. Tsunoda", "team_id": 6, "race_number": 22, "nationality": 43},
    {"name": "I. Hadjar", "team_id": 6, "race_number": 6, "nationality": 28},
    {"name": "A. Albon", "team_id": 3, "race_number": 23, "nationality": 80},
    {"name": "C. Sainz", "team_id": 3, "race_number": 55, "nationality": 77},
    {"name": "N. Hulkenberg", "team_id": 9, "race_number": 27, "nationality": 29},
    {"name": "G. Bortoleto", "team_id": 9, "race_number": 5, "nationality": 9},
    {"name": "O. Bearman", "team_id": 7, "race_number": 87, "nationality": 10},
    {"name": "E. Ocon", "team_id": 7, "race_number": 31, "nationality": 28},
]

# Track characteristics: corners at specific percentages of the track
CORNERS = [0.15, 0.35, 0.50, 0.80, 0.90]


class BaseTelemetryParser:
    """
    Simulates realistic live F1 25 UDP telemetry data across all 16 packet types.
    Emits packets at frequencies matching the real F1 game.
    """

    def __init__(self):
        self._is_running = False
        self._frame = 0
        self._session_uid = random.randint(100000, 999999)
        self._start_time = 0.0

        # Race state
        self._lap = 1
        self._track_pos = 0.0
        self._total_laps = 57
        self._track_length = 5412  # Monza-ish
        self._track_id = 11  # Monza
        self._session_type = 10  # Race

        # Car state (player = index 0)
        self._num_cars = 20
        self._player_idx = 0
        self._grid_position = random.randint(5, 15)

        # Tire state
        self._wear = [5.0, 5.0, 5.0, 5.0]  # [RL, RR, FL, FR]
        self._tyre_compound = TyreCompound.C3
        self._tyre_visual = TyreCompoundVisual.HARD
        self._tyre_age_laps = 0
        self._has_pitted = False

        # Fuel state
        self._fuel_in_tank = 100.0
        self._fuel_capacity = 110.0
        self._fuel_burn_rate = 1.8  # kg per lap

        # ERS state
        self._ers_store = 4000000.0  # 4 MJ max
        self._ers_deploy_mode = ERSDeployMode.MEDIUM
        self._ers_harvested_mguk = 0.0
        self._ers_harvested_mguh = 0.0
        self._ers_deployed = 0.0

        # Weather state
        self._weather = Weather.CLEAR
        self._track_temp = 42
        self._air_temp = 28
        self._rain_percentage = 0

        # Safety car
        self._safety_car = SafetyCarStatus.NONE

        # Damage state
        self._damage_fl_wing = 0
        self._damage_fr_wing = 0
        self._damage_rear_wing = 0
        self._damage_floor = 0
        self._damage_gearbox = 0
        self._damage_engine = 0

        # Lap timing
        self._sector_times = [0, 0, 0]  # ms for current lap
        self._last_lap_time = 0
        self._best_lap_time = 0
        self._sector_start_time = 0.0
        self._lap_history: List[LapHistoryData] = []

        # AI car positions (simulated)
        self._car_positions = list(range(1, self._num_cars + 1))
        random.shuffle(self._car_positions)
        self._car_positions[self._player_idx] = self._grid_position

        # Pit stop decision (decide once, not every tick)
        self._pit_lap = random.randint(20, 25)

        # Event tracking
        self._lights_out_sent = False
        self._last_event_time = 0.0

    def _make_header(self, packet_id: int) -> PacketHeader:
        return PacketHeader(
            packet_format=2025,
            game_year=25,
            game_major_version=1,
            game_minor_version=0,
            packet_version=1,
            packet_id=packet_id,
            session_uid=self._session_uid,
            session_time=time.time() - self._start_time,
            frame_identifier=self._frame,
            overall_frame_identifier=self._frame,
            player_car_index=self._player_idx,
        )

    async def start(self):
        self._is_running = True
        self._start_time = time.time()
        logger.info(
            "Telemetry Parser (Full Simulation) started - emitting all 16 packet types."
        )
        await self._listen_loop()

    def stop(self):
        self._is_running = False
        logger.info("Telemetry Parser stopped.")

    async def _listen_loop(self):
        """Main simulation loop at 20Hz. Emits different packets at appropriate intervals."""
        tick_count = 0
        prev_lap = self._lap

        while self._is_running:
            await asyncio.sleep(0.05)  # 20Hz
            self._frame += 1
            tick_count += 1

            # --- Update physics ---
            self._update_car_physics()

            # --- Lap completion ---
            if self._lap > prev_lap:
                self._on_lap_complete(prev_lap)
                await self.emit_session_history()
                prev_lap = self._lap

            # --- Emit packets at appropriate frequencies ---

            # 20Hz packets: motion, lap_data, car_telemetry, car_status, motion_ex
            await self._emit_car_telemetry()
            await self._emit_lap_data()
            await self._emit_car_status()

            # Motion at 20Hz but only every other tick to reduce load (effective 10Hz)
            if tick_count % 2 == 0:
                await self._emit_motion()
                await self._emit_motion_ex()

            # 10Hz: car_damage
            if tick_count % 2 == 0:
                await self._emit_car_damage()

            # 2Hz: session (every 10 ticks)
            if tick_count % 10 == 0:
                await self._emit_session()

            # Every 5s (100 ticks): participants, car_setup, tyre_sets
            if tick_count % 100 == 0:
                await self._emit_participants()
                await self._emit_car_setup()
                await self._emit_tyre_sets()

            # Lights out event at the start
            if not self._lights_out_sent and tick_count == 20:
                await self._emit_event(EventCode.LIGHTS_OUT)
                self._lights_out_sent = True

            # Random events
            if tick_count % 200 == 0:
                await self._maybe_emit_random_event()

    def _update_car_physics(self):
        """Update all car physics state for one tick."""
        # Move the car forward
        self._track_pos += 0.001

        # Lap completion
        if self._track_pos >= 1.0:
            self._track_pos = 0.0
            self._lap += 1
            self._tyre_age_laps += 1
            self._ers_harvested_mguk = 0.0
            self._ers_harvested_mguh = 0.0
            self._ers_deployed = 0.0

        # Distance to nearest corner
        dist_to_corner = min(abs(self._track_pos - c) for c in CORNERS)

        # Tire wear (scaled to produce ~50-60% wear by lap 20-25)
        # ~1000 ticks/lap, ~15% in corners, ~10% in braking zones, ~75% on straights
        # Target: ~2.5% wear per lap on rear tires
        in_corner = dist_to_corner < 0.03
        in_braking = 0.03 <= dist_to_corner < 0.05
        if in_corner:
            wear_rate = 0.005
        elif in_braking:
            wear_rate = 0.003
        else:
            wear_rate = 0.001
        self._wear[0] += wear_rate * 1.5  # RL (highest wear)
        self._wear[1] += wear_rate * 1.3  # RR
        self._wear[2] += wear_rate * 1.1  # FL
        self._wear[3] += wear_rate * 1.0  # FR

        # Fuel consumption (tiny amount per tick)
        fuel_per_tick = self._fuel_burn_rate / 1000.0  # ~1000 ticks per lap
        self._fuel_in_tank = max(0.0, self._fuel_in_tank - fuel_per_tick)

        # ERS: harvest in braking zones, deploy on straights
        if in_corner:
            self._ers_harvested_mguk += 800
            self._ers_harvested_mguh += 500
            self._ers_store = min(4000000.0, self._ers_store + 1300)
        elif dist_to_corner > 0.08:  # straight
            deploy = 2000 if self._ers_deploy_mode == ERSDeployMode.OVERTAKE else 1000
            self._ers_deployed += deploy
            self._ers_store = max(0.0, self._ers_store - deploy)

        # Damage accumulation (very slow, random micro-damage)
        if random.random() < 0.0001:
            target = random.choice(["fl_wing", "fr_wing", "floor", "gearbox"])
            if target == "fl_wing":
                self._damage_fl_wing = min(
                    100, self._damage_fl_wing + random.randint(1, 3)
                )
            elif target == "fr_wing":
                self._damage_fr_wing = min(
                    100, self._damage_fr_wing + random.randint(1, 3)
                )
            elif target == "floor":
                self._damage_floor = min(100, self._damage_floor + random.randint(1, 2))
            elif target == "gearbox":
                self._damage_gearbox = min(
                    100, self._damage_gearbox + random.randint(1, 2)
                )

        # Weather evolution (very slow drift)
        if random.random() < 0.0002:
            self._track_temp = max(
                28, min(55, self._track_temp + random.choice([-1, 1]))
            )
            self._air_temp = max(
                20, min(38, self._air_temp + random.choice([-1, 0, 1]))
            )

        # Pit stop simulation (auto-pit at pre-decided lap)
        if (
            not self._has_pitted
            and self._lap >= self._pit_lap
            and self._track_pos < 0.02
        ):
            self._do_pit_stop()

    def _on_lap_complete(self, prev_lap: int):
        """Handle lap completion: update history, emit history packet."""
        # Generate realistic sector times
        s1 = random.randint(27000, 29000)
        s2 = random.randint(34000, 36000)
        s3 = random.randint(24000, 26000)
        lap_time = s1 + s2 + s3

        self._last_lap_time = lap_time
        if self._best_lap_time == 0 or lap_time < self._best_lap_time:
            self._best_lap_time = lap_time

        self._lap_history.append(
            LapHistoryData(
                lap_time_in_ms=lap_time,
                sector1_time_in_ms=s1,
                sector1_time_minutes=0,
                sector2_time_in_ms=s2,
                sector2_time_minutes=0,
                sector3_time_in_ms=s3,
                sector3_time_minutes=0,
                lap_valid_bit_flags=0x0F,
            )
        )

        # Evolve car positions slightly
        for i in range(1, self._num_cars):
            if random.random() < 0.1:
                j = random.randint(1, self._num_cars - 1)
                if i != j:
                    self._car_positions[i], self._car_positions[j] = (
                        self._car_positions[j],
                        self._car_positions[i],
                    )

        # Player position evolution (slowly improve)
        if random.random() < 0.15 and self._car_positions[self._player_idx] > 1:
            self._car_positions[self._player_idx] -= 1

        logger.info(
            f"Lap {prev_lap} complete: {lap_time / 1000:.3f}s (P{self._car_positions[self._player_idx]})"
        )

    def _do_pit_stop(self):
        """Simulate a pit stop - reset tires and refuel."""
        self._has_pitted = True
        self._wear = [2.0, 2.0, 2.0, 2.0]
        self._tyre_compound = TyreCompound.C4
        self._tyre_visual = TyreCompoundVisual.MEDIUM
        self._tyre_age_laps = 0
        logger.info("PIT STOP: Switched to Medium compound.")

    def _get_driving_state(self):
        """Calculate current driving physics based on track position."""
        dist_to_corner = min(abs(self._track_pos - c) for c in CORNERS)

        if dist_to_corner < 0.03:
            # Corner Apex (already slow)
            speed = 110 + random.randint(-10, 10)
            gear = 3
            throttle = 0.1
            brake = 0.6 + random.uniform(0, 0.2)
            rpm = 7500 + random.randint(-500, 500)
            steer = 0.8 * (1 if self._track_pos > 0.5 else -1)
            g_lat = random.uniform(2.5, 3.5) * (1 if self._track_pos > 0.5 else -1)
            g_long = random.uniform(-2.0, -0.5)
            drs = 0
        elif dist_to_corner < 0.05:
            # Heavy braking zone (approaching corner - still high speed)
            speed = 260 + random.randint(-15, 15)
            gear = 5
            throttle = 0.0
            brake = 0.85 + random.uniform(0, 0.15)
            rpm = 9000 + random.randint(-500, 500)
            steer = 0.3 * (1 if self._track_pos > 0.5 else -1)
            g_lat = random.uniform(0.5, 1.5)
            g_long = random.uniform(-5.0, -3.5)
            drs = 0
        elif dist_to_corner < 0.08:
            # Acceleration Zone
            speed = 220 + random.randint(-15, 15)
            gear = 5
            throttle = 0.8
            brake = 0.0
            rpm = 10500 + random.randint(-500, 500)
            steer = 0.2
            g_lat = random.uniform(0.5, 1.5)
            g_long = random.uniform(0.5, 1.5)
            drs = 0
        else:
            # Straight
            speed = 315 + random.randint(-5, 5)
            gear = 8
            throttle = 1.0
            brake = 0.0
            rpm = 11800 + random.randint(-200, 200)
            steer = 0.0
            g_lat = random.uniform(-0.2, 0.2)
            g_long = random.uniform(0.3, 0.8)
            drs = 1 if self._lap > 2 else 0

        return {
            "speed": speed,
            "gear": gear,
            "throttle": throttle,
            "brake": brake,
            "rpm": rpm,
            "steer": steer,
            "g_lat": g_lat,
            "g_long": g_long,
            "drs": drs,
        }

    # --- Packet emission methods ---

    async def _emit_car_telemetry(self):
        """Packet 6: Car Telemetry Data - 20Hz."""
        ds = self._get_driving_state()

        # Calculate tire temperatures based on driving
        if ds["brake"] > 0.8:
            base_surface = 115  # Heavy braking heats tires significantly
            base_inner = 108
            brake_temp_base = 850
        elif ds["brake"] > 0.4:
            base_surface = 105
            base_inner = 103
            brake_temp_base = 650
        else:
            base_surface = 95
            base_inner = 100
            brake_temp_base = 400

        player_telemetry = CarTelemetry(
            speed=ds["speed"],
            throttle=ds["throttle"],
            steer=ds["steer"],
            brake=ds["brake"],
            clutch=0,
            gear=ds["gear"],
            engine_rpm=ds["rpm"],
            drs=ds["drs"],
            rev_lights_percent=min(100, int(ds["rpm"] / 130)),
            rev_lights_bit_value=0,
            brakes_temperature=[
                brake_temp_base + random.randint(-30, 50),
                brake_temp_base + random.randint(-30, 50),
                brake_temp_base + random.randint(-20, 60),
                brake_temp_base + random.randint(-20, 60),
            ],
            tyres_surface_temperature=[
                base_surface + random.randint(-5, 10),
                base_surface + random.randint(-5, 10),
                base_surface + random.randint(-3, 8),
                base_surface + random.randint(-3, 8),
            ],
            tyres_inner_temperature=[
                base_inner + random.randint(-3, 5),
                base_inner + random.randint(-3, 5),
                base_inner + random.randint(-2, 4),
                base_inner + random.randint(-2, 4),
            ],
            engine_temperature=100 + random.randint(-5, 15),
            tyres_pressure=[
                22.0 + random.uniform(-0.3, 0.5),
                22.0 + random.uniform(-0.3, 0.5),
                23.5 + random.uniform(-0.3, 0.5),
                23.5 + random.uniform(-0.3, 0.5),
            ],
            surface_type=[0, 0, 0, 0],
        )

        # Generate simplified AI car telemetry
        cars = [player_telemetry]
        for i in range(1, self._num_cars):
            ai_speed = random.randint(100, 320)
            cars.append(
                CarTelemetry(
                    speed=ai_speed,
                    throttle=random.uniform(0, 1),
                    steer=random.uniform(-0.5, 0.5),
                    brake=random.uniform(0, 0.5),
                    gear=random.randint(2, 8),
                    engine_rpm=random.randint(6000, 12000),
                    drs=random.choice([0, 1]),
                    brakes_temperature=[random.randint(300, 800)] * 4,
                    tyres_surface_temperature=[random.randint(85, 115)] * 4,
                    tyres_inner_temperature=[random.randint(90, 110)] * 4,
                    engine_temperature=random.randint(95, 115),
                    tyres_pressure=[random.uniform(21.5, 24.0)] * 4,
                )
            )
        # Pad to 22
        while len(cars) < 22:
            cars.append(CarTelemetry())

        packet = PacketCarTelemetryData(
            header=self._make_header(6),
            car_telemetry_data=cars,
            suggested_gear=ds["gear"],
        )
        await bus.publish("packet_car_telemetry", packet)

    async def _emit_lap_data(self):
        """Packet 2: Lap Data - 20Hz."""
        elapsed = time.time() - self._start_time
        sector = 0
        if self._track_pos > 0.33:
            sector = 1
        if self._track_pos > 0.66:
            sector = 2

        # Current lap time estimate
        current_lap_time = int((self._track_pos) * 90000)  # ~90s lap

        player_lap = CarLapData(
            last_lap_time_in_ms=self._last_lap_time,
            current_lap_time_in_ms=current_lap_time,
            sector1_time_in_ms=self._sector_times[0]
            if sector > 0
            else current_lap_time,
            sector2_time_in_ms=self._sector_times[1] if sector > 1 else 0,
            lap_distance=self._track_pos * self._track_length,
            total_distance=(self._lap - 1) * self._track_length
            + self._track_pos * self._track_length,
            car_position=self._car_positions[self._player_idx],
            current_lap_num=self._lap,
            pit_status=0,
            num_pit_stops=1 if self._has_pitted else 0,
            sector=sector,
            grid_position=self._grid_position,
            driver_status=4,  # ON_TRACK
            result_status=2,  # ACTIVE
            delta_to_car_in_front_in_ms=random.randint(200, 3000),
            delta_to_race_leader_in_ms=random.randint(0, 15000)
            if self._car_positions[self._player_idx] > 1
            else 0,
            speed_trap_fastest_speed=320.0 + random.uniform(-5, 10),
        )

        # Generate AI lap data
        cars = [player_lap]
        for i in range(1, self._num_cars):
            cars.append(
                CarLapData(
                    current_lap_num=self._lap + random.choice([-1, 0, 0, 0]),
                    car_position=self._car_positions[i]
                    if i < len(self._car_positions)
                    else i + 1,
                    last_lap_time_in_ms=random.randint(85000, 95000),
                    sector=random.randint(0, 2),
                    grid_position=i + 1,
                    driver_status=4,
                    result_status=2,
                    delta_to_car_in_front_in_ms=random.randint(200, 5000),
                )
            )
        while len(cars) < 22:
            cars.append(CarLapData())

        # Update sector times for next iteration
        if sector == 0:
            self._sector_times = [current_lap_time, 0, 0]
        elif sector == 1:
            self._sector_times[0] = random.randint(27000, 29000)

        packet = PacketLapData(header=self._make_header(2), car_lap_data=cars)
        await bus.publish("packet_lap_data", packet)

    async def _emit_car_status(self):
        """Packet 7: Car Status Data - 20Hz."""
        fuel_remaining_laps = (
            self._fuel_in_tank / self._fuel_burn_rate if self._fuel_burn_rate > 0 else 0
        )

        player_status = CarStatus(
            fuel_mix=FuelMix.STANDARD,
            front_brake_bias=56,
            fuel_in_tank=self._fuel_in_tank,
            fuel_capacity=self._fuel_capacity,
            fuel_remaining_laps=fuel_remaining_laps,
            max_rpm=13000,
            idle_rpm=3500,
            max_gears=8,
            drs_allowed=1 if self._lap > 2 and self._track_pos > 0.08 else 0,
            actual_tyre_compound=int(self._tyre_compound),
            visual_tyre_compound=int(self._tyre_visual),
            tyres_age_laps=self._tyre_age_laps,
            vehicle_fia_flags=0,
            engine_power_ice=750.0 + random.uniform(-5, 5),
            engine_power_mguk=120.0 + random.uniform(-2, 2),
            ers_store_energy=self._ers_store,
            ers_deploy_mode=int(self._ers_deploy_mode),
            ers_harvested_this_lap_mguk=self._ers_harvested_mguk,
            ers_harvested_this_lap_mguh=self._ers_harvested_mguh,
            ers_deployed_this_lap=self._ers_deployed,
        )

        cars = [player_status]
        for i in range(1, self._num_cars):
            cars.append(
                CarStatus(
                    fuel_in_tank=random.uniform(20, 100),
                    fuel_remaining_laps=random.uniform(5, 50),
                    actual_tyre_compound=random.choice([16, 17, 18]),
                    visual_tyre_compound=random.choice([16, 17, 18]),
                    tyres_age_laps=random.randint(0, 30),
                    ers_store_energy=random.uniform(0, 4000000),
                    ers_deploy_mode=random.randint(0, 3),
                )
            )
        while len(cars) < 22:
            cars.append(CarStatus())

        packet = PacketCarStatusData(header=self._make_header(7), car_status_data=cars)
        await bus.publish("packet_car_status", packet)

    async def _emit_motion(self):
        """Packet 0: Motion Data - 10Hz effective."""
        ds = self._get_driving_state()

        # Simulate world position along a circular-ish track
        angle = self._track_pos * 2 * math.pi
        radius = 500.0
        player_motion = CarMotionData(
            world_position_x=radius * math.cos(angle),
            world_position_y=5.0,
            world_position_z=radius * math.sin(angle),
            world_velocity_x=ds["speed"] * math.cos(angle + math.pi / 2) / 3.6,
            world_velocity_y=0.0,
            world_velocity_z=ds["speed"] * math.sin(angle + math.pi / 2) / 3.6,
            g_force_lateral=ds["g_lat"],
            g_force_longitudinal=ds["g_long"],
            g_force_vertical=1.0 + random.uniform(-0.1, 0.1),
            yaw=angle,
            pitch=random.uniform(-0.02, 0.02),
            roll=random.uniform(-0.05, 0.05),
        )

        cars = [player_motion]
        for i in range(1, self._num_cars):
            ai_angle = (self._track_pos + random.uniform(-0.1, 0.1)) * 2 * math.pi
            cars.append(
                CarMotionData(
                    world_position_x=radius * math.cos(ai_angle)
                    + random.uniform(-5, 5),
                    world_position_z=radius * math.sin(ai_angle)
                    + random.uniform(-5, 5),
                    g_force_lateral=random.uniform(-2, 2),
                    g_force_longitudinal=random.uniform(-3, 1),
                )
            )
        while len(cars) < 22:
            cars.append(CarMotionData())

        packet = PacketMotionData(header=self._make_header(0), car_motion_data=cars)
        await bus.publish("packet_motion", packet)

    async def _emit_motion_ex(self):
        """Packet 13: Motion Ex Data (player car only) - 10Hz."""
        ds = self._get_driving_state()
        speed_ms = ds["speed"] / 3.6

        packet = PacketMotionExData(
            header=self._make_header(13),
            suspension_position=[random.uniform(-5, 5) for _ in range(4)],
            suspension_velocity=[random.uniform(-50, 50) for _ in range(4)],
            suspension_acceleration=[random.uniform(-500, 500) for _ in range(4)],
            wheel_speed=[speed_ms + random.uniform(-2, 2) for _ in range(4)],
            wheel_slip_ratio=[random.uniform(-0.05, 0.05) for _ in range(4)],
            wheel_slip_angle=[random.uniform(-3, 3) for _ in range(4)],
            wheel_lat_force=[random.uniform(-5000, 5000) for _ in range(4)],
            wheel_long_force=[random.uniform(-8000, 8000) for _ in range(4)],
            wheel_vert_force=[random.uniform(3000, 6000) for _ in range(4)],
            height_of_cog_above_ground=0.3 + random.uniform(-0.02, 0.02),
            front_aero_height=0.05 + random.uniform(-0.01, 0.01),
            rear_aero_height=0.08 + random.uniform(-0.01, 0.01),
            local_velocity_x=speed_ms,
            local_velocity_y=random.uniform(-0.5, 0.5),
            local_velocity_z=random.uniform(-1, 1),
            front_wheels_angle=ds["steer"] * 0.3,
        )
        await bus.publish("packet_motion_ex", packet)

    async def _emit_car_damage(self):
        """Packet 10: Car Damage Data - 10Hz."""
        player_damage = CarDamage(
            tyres_wear=list(self._wear),  # [RL, RR, FL, FR]
            tyres_damage=[0, 0, 0, 0],
            brakes_damage=[random.randint(0, 5)] * 4,
            front_left_wing_damage=self._damage_fl_wing,
            front_right_wing_damage=self._damage_fr_wing,
            rear_wing_damage=self._damage_rear_wing,
            floor_damage=self._damage_floor,
            diffuser_damage=0,
            sidepod_damage=0,
            gear_box_damage=self._damage_gearbox,
            engine_damage=self._damage_engine,
            engine_mguh_wear=random.randint(0, 10),
            engine_es_wear=random.randint(0, 10),
            engine_ce_wear=random.randint(0, 10),
            engine_ice_wear=random.randint(0, 15),
            engine_mguk_wear=random.randint(0, 10),
            engine_tc_wear=random.randint(0, 10),
        )

        cars = [player_damage]
        for i in range(1, self._num_cars):
            cars.append(
                CarDamage(
                    tyres_wear=[random.uniform(5, 60) for _ in range(4)],
                    front_left_wing_damage=random.randint(0, 20),
                    front_right_wing_damage=random.randint(0, 20),
                )
            )
        while len(cars) < 22:
            cars.append(CarDamage())

        packet = PacketCarDamageData(header=self._make_header(10), car_damage_data=cars)
        await bus.publish("packet_car_damage", packet)

    async def _emit_session(self):
        """Packet 1: Session Data - 2Hz."""
        elapsed = time.time() - self._start_time
        time_left = max(0, 7200 - int(elapsed))

        # Weather forecasts
        forecasts = []
        for offset in [0, 5, 10, 15, 30]:
            rain = self._rain_percentage
            if offset > 10:
                rain = min(100, rain + random.randint(0, 15))
            forecasts.append(
                WeatherForecastSample(
                    session_type=self._session_type,
                    time_offset=offset,
                    weather=int(self._weather),
                    track_temperature=self._track_temp + random.randint(-2, 2),
                    air_temperature=self._air_temp + random.randint(-1, 1),
                    rain_percentage=rain,
                )
            )

        # Pit window (simple: middle third of race)
        ideal_pit = max(1, self._total_laps // 3)
        latest_pit = min(self._total_laps, self._total_laps * 2 // 3)

        packet = PacketSessionData(
            header=self._make_header(1),
            weather=int(self._weather),
            track_temperature=self._track_temp,
            air_temperature=self._air_temp,
            total_laps=self._total_laps,
            track_length=self._track_length,
            session_type=self._session_type,
            track_id=self._track_id,
            session_time_left=time_left,
            session_duration=7200,
            pit_speed_limit=80,
            safety_car_status=int(self._safety_car),
            num_weather_forecast_samples=len(forecasts),
            weather_forecast_samples=forecasts,
            forecast_accuracy=0,
            ai_difficulty=90,
            pit_stop_window_ideal_lap=ideal_pit,
            pit_stop_window_latest_lap=latest_pit,
            num_safety_car_periods=0,
            num_virtual_safety_car_periods=0,
            num_red_flag_periods=0,
            sector2_lap_distance_start=self._track_length * 0.33,
            sector3_lap_distance_start=self._track_length * 0.66,
        )
        await bus.publish("packet_session", packet)

    async def _emit_participants(self):
        """Packet 4: Participants Data - every 5s."""
        parts = []
        for i, driver in enumerate(AI_DRIVERS):
            parts.append(
                ParticipantData(
                    ai_controlled=0 if i == self._player_idx else 1,
                    driver_id=i,
                    team_id=driver["team_id"],
                    race_number=driver["race_number"],
                    nationality=driver["nationality"],
                    name=driver["name"],
                    your_telemetry=1,
                )
            )
        while len(parts) < 22:
            parts.append(ParticipantData(name="", ai_controlled=1))

        packet = PacketParticipantsData(
            header=self._make_header(4),
            num_active_cars=self._num_cars,
            participants=parts,
        )
        await bus.publish("packet_participants", packet)

    async def _emit_car_setup(self):
        """Packet 5: Car Setup Data - every 5s."""
        setups = []
        for i in range(22):
            setups.append(
                CarSetup(
                    front_wing=random.randint(3, 10),
                    rear_wing=random.randint(3, 10),
                    brake_pressure=random.randint(85, 100),
                    brake_bias=random.randint(52, 60),
                    fuel_load=self._fuel_in_tank
                    if i == self._player_idx
                    else random.uniform(20, 100),
                )
            )
        packet = PacketCarSetupData(header=self._make_header(5), car_setups=setups)
        await bus.publish("packet_car_setup", packet)

    async def _emit_tyre_sets(self):
        """Packet 12: Tyre Sets Data - every 5s."""
        sets = []
        # 3 sets of each dry compound + 1 inter + 1 wet = up to 20
        compounds = [
            (16, 16),
            (16, 16),
            (16, 16),  # Soft
            (17, 17),
            (17, 17),
            (17, 17),  # Medium
            (18, 18),
            (18, 18),
            (18, 18),  # Hard
            (7, 7),
            (7, 7),  # Inter
            (8, 8),
            (8, 8),  # Wet
        ]
        for idx, (actual, visual) in enumerate(compounds):
            fitted = 1 if actual == int(self._tyre_compound) and idx == 0 else 0
            sets.append(
                TyreSetData(
                    actual_tyre_compound=actual,
                    visual_tyre_compound=visual,
                    wear=int(max(self._wear)) if fitted else random.randint(0, 30),
                    available=1,
                    life_span=100 - random.randint(0, 30),
                    usable_life=70 - random.randint(0, 20),
                    fitted=fitted,
                )
            )
        while len(sets) < 20:
            sets.append(TyreSetData(available=0))

        packet = PacketTyreSetsData(
            header=self._make_header(12),
            car_idx=self._player_idx,
            tyre_set_data=sets,
        )
        await bus.publish("packet_tyre_sets", packet)

    async def _emit_event(self, event_code: str, **details):
        """Packet 3: Event Data - on occurrence."""
        packet = PacketEventData(
            header=self._make_header(3),
            event_string_code=event_code,
            event_details=EventDataDetails(**details),
        )
        await bus.publish("packet_event", packet)

    async def _maybe_emit_random_event(self):
        """Occasionally emit random race events."""
        events = [
            (
                EventCode.SPEED_TRAP,
                {
                    "vehicle_idx": self._player_idx,
                    "speed": 320.0 + random.uniform(-10, 15),
                },
            ),
            (
                EventCode.OVERTAKE,
                {
                    "overtaking_vehicle_idx": random.randint(0, self._num_cars - 1),
                    "being_overtaken_vehicle_idx": random.randint(
                        0, self._num_cars - 1
                    ),
                },
            ),
        ]
        if self._lap > 2 and random.random() < 0.3:
            events.append(
                (
                    EventCode.FASTEST_LAP,
                    {
                        "vehicle_idx": random.randint(0, self._num_cars - 1),
                        "lap_time": random.uniform(85.0, 90.0),
                    },
                )
            )

        code, details = random.choice(events)
        await self._emit_event(code, **details)

    async def emit_session_history(self):
        """Packet 11: Session History - called on lap completion."""
        if not self._lap_history:
            return

        best_lap_num = 0
        best_time = float("inf")
        for idx, lh in enumerate(self._lap_history):
            if lh.lap_time_in_ms < best_time:
                best_time = lh.lap_time_in_ms
                best_lap_num = idx + 1

        packet = PacketSessionHistoryData(
            header=self._make_header(11),
            car_idx=self._player_idx,
            num_laps=len(self._lap_history),
            num_tyre_stints=2 if self._has_pitted else 1,
            best_lap_time_lap_num=best_lap_num,
            lap_history_data=list(self._lap_history),
            tyre_stint_history_data=[
                TyreStintHistoryData(
                    end_lap=20 if self._has_pitted else 0,
                    tyre_actual_compound=int(TyreCompound.C3),
                    tyre_visual_compound=int(TyreCompoundVisual.HARD),
                ),
            ],
        )
        await bus.publish("packet_session_history", packet)
