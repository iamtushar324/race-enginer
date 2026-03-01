"""
Real F1 25 UDP Telemetry Parser.
Listens for UDP packets from the F1 25 game, parses binary data using ctypes,
converts to Pydantic models, and publishes on the event bus.
"""

import asyncio
import ctypes
import importlib.util
import logging
import os
import socket
import time
from typing import Optional

from race_engineer.core.event_bus import bus
from race_engineer.telemetry import packets

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Dynamically load ctypes packet definitions from the reference F1 25 parser
# ---------------------------------------------------------------------------
def _load_ctypes_module():
    parser_path = os.path.join(
        os.path.dirname(__file__),
        "..",
        "..",
        "f1-25-telemetry-application",
        "src",
        "parsers",
        "parser2025.py",
    )
    parser_path = os.path.abspath(parser_path)
    if not os.path.exists(parser_path):
        logger.warning(f"Reference F1 25 parser not found at {parser_path}")
        return None
    spec = importlib.util.spec_from_file_location("_f1_ctypes", parser_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


_ct = _load_ctypes_module()


# ---------------------------------------------------------------------------
# Converter helpers: ctypes → Pydantic
# ---------------------------------------------------------------------------


def _convert_header(h) -> packets.PacketHeader:
    return packets.PacketHeader(
        packet_format=h.m_packet_format,
        game_year=h.m_game_year,
        game_major_version=h.m_game_major_version,
        game_minor_version=h.m_game_minor_version,
        packet_version=h.m_packet_version,
        packet_id=h.m_packet_id,
        session_uid=h.m_session_uid,
        session_time=h.m_session_time,
        frame_identifier=h.m_frame_identifier,
        overall_frame_identifier=h.m_overall_frame_identifier,
        player_car_index=h.m_player_car_index,
        secondary_player_car_index=h.m_secondary_player_car_index,
    )


def _convert_motion(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    cars = []
    for i in range(22):
        c = ct_pkt.m_car_motion_data[i]
        cars.append(
            packets.CarMotionData(
                world_position_x=c.m_world_position_x,
                world_position_y=c.m_world_position_y,
                world_position_z=c.m_world_position_z,
                world_velocity_x=c.m_world_velocity_x,
                world_velocity_y=c.m_world_velocity_y,
                world_velocity_z=c.m_world_velocity_z,
                world_forward_dir_x=c.m_world_forward_dir_x,
                world_forward_dir_y=c.m_world_forward_dir_y,
                world_forward_dir_z=c.m_world_forward_dir_z,
                world_right_dir_x=c.m_world_right_dir_x,
                world_right_dir_y=c.m_world_right_dir_y,
                world_right_dir_z=c.m_world_right_dir_z,
                g_force_lateral=c.m_g_force_lateral,
                g_force_longitudinal=c.m_g_force_longitudinal,
                g_force_vertical=c.m_g_force_vertical,
                yaw=c.m_yaw,
                pitch=c.m_pitch,
                roll=c.m_roll,
            )
        )
    return packets.PacketMotionData(header=header, car_motion_data=cars)


def _convert_session(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    marshal_zones = []
    for i in range(ct_pkt.m_num_marshal_zones):
        mz = ct_pkt.m_marshal_zones[i]
        marshal_zones.append(
            packets.MarshalZone(
                zone_start=mz.m_zone_start,
                zone_flag=mz.m_zone_flag,
            )
        )
    forecasts = []
    for i in range(min(ct_pkt.m_num_weather_forecast_samples, 64)):
        ws = ct_pkt.m_weather_forecast_samples[i]
        forecasts.append(
            packets.WeatherForecastSample(
                session_type=ws.m_session_type,
                time_offset=ws.m_time_offset,
                weather=ws.m_weather,
                track_temperature=ws.m_track_temperature,
                track_temperature_change=ws.m_track_temperature_change,
                air_temperature=ws.m_air_temperature,
                air_temperature_change=ws.m_air_temperature_change,
                rain_percentage=ws.m_rain_percentage,
            )
        )
    return packets.PacketSessionData(
        header=header,
        weather=ct_pkt.m_weather,
        track_temperature=ct_pkt.m_track_temperature,
        air_temperature=ct_pkt.m_air_temperature,
        total_laps=ct_pkt.m_total_laps,
        track_length=ct_pkt.m_track_length,
        session_type=ct_pkt.m_session_type,
        track_id=ct_pkt.m_track_id,
        formula=ct_pkt.m_formula,
        session_time_left=ct_pkt.m_session_time_left,
        session_duration=ct_pkt.m_session_duration,
        pit_speed_limit=ct_pkt.m_pit_speed_limit,
        game_paused=ct_pkt.m_game_paused,
        is_spectating=ct_pkt.m_is_spectating,
        num_marshal_zones=ct_pkt.m_num_marshal_zones,
        marshal_zones=marshal_zones,
        safety_car_status=ct_pkt.m_safety_car_status,
        network_game=ct_pkt.m_network_game,
        num_weather_forecast_samples=len(forecasts),
        weather_forecast_samples=forecasts,
        forecast_accuracy=ct_pkt.m_forecast_accuracy,
        ai_difficulty=ct_pkt.m_ai_difficulty,
        pit_stop_window_ideal_lap=ct_pkt.m_pit_stop_window_ideal_lap,
        pit_stop_window_latest_lap=ct_pkt.m_pit_stop_window_latest_lap,
        pit_stop_rejoin_position=ct_pkt.m_pit_stop_rejoin_position,
        num_safety_car_periods=ct_pkt.m_num_safety_car_periods,
        num_virtual_safety_car_periods=ct_pkt.m_num_virtual_safety_car_periods,
        num_red_flag_periods=ct_pkt.m_num_red_flag_periods,
        sector2_lap_distance_start=ct_pkt.m_sector2LapDistanceStart,
        sector3_lap_distance_start=ct_pkt.m_sector3LapDistanceStart,
    )


def _convert_lap_data(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    cars = []
    for i in range(22):
        c = ct_pkt.m_lap_data[i]
        delta_front = (
            c.m_deltaToCarInFrontMinutesPart * 60000 + c.m_deltaToCarInFrontMSPart
        )
        delta_leader = (
            c.m_deltaToRaceLeaderMinutesPart * 60000 + c.m_deltaToRaceLeaderMSPart
        )
        cars.append(
            packets.CarLapData(
                last_lap_time_in_ms=c.m_last_lap_time_in_ms,
                current_lap_time_in_ms=c.m_current_lap_time_in_ms,
                sector1_time_in_ms=c.m_sector1_time_in_ms,
                sector1_time_minutes=c.m_sector1_time_in_minutes,
                sector2_time_in_ms=c.m_sector2_time_in_ms,
                sector2_time_minutes=c.m_sector2_time_in_minutes,
                delta_to_car_in_front_in_ms=delta_front,
                delta_to_race_leader_in_ms=delta_leader,
                lap_distance=c.m_lap_distance,
                total_distance=c.m_total_distance,
                safety_car_delta=c.m_safety_car_delta,
                car_position=c.m_car_position,
                current_lap_num=c.m_current_lap_num,
                pit_status=c.m_pit_status,
                num_pit_stops=c.m_num_pit_stops,
                sector=c.m_sector,
                current_lap_invalid=c.m_current_lap_invalid,
                penalties=c.m_penalties,
                total_warnings=c.m_total_warnings,
                corner_cutting_warnings=c.m_corner_cutting_warnings,
                num_unserved_drive_through_pens=c.m_num_unserved_drive_through_pens,
                num_unserved_stop_go_pens=c.m_num_unserved_stop_go_pens,
                grid_position=c.m_grid_position,
                driver_status=c.m_driver_status,
                result_status=c.m_result_status,
                pit_lane_timer_active=c.m_pit_lane_timer_active,
                pit_lane_time_in_lane_in_ms=c.m_pit_lane_time_in_lane_in_ms,
                pit_stop_timer_in_ms=c.m_pit_stop_timer_in_ms,
                pit_stop_should_serve_pen=c.m_pit_stop_should_serve_pen,
                speed_trap_fastest_speed=c.m_speedTrapFastestSpeed,
                speed_trap_fastest_lap=c.m_speedTrapFastestLap,
            )
        )
    return packets.PacketLapData(header=header, car_lap_data=cars)


def _convert_event(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    code_bytes = bytes(ct_pkt.m_event_string_code)
    event_code = code_bytes.decode("ascii", errors="replace").rstrip("\x00")

    details = packets.EventDataDetails()
    ed = ct_pkt.m_event_details

    if event_code == "FTLP":
        fl = ed.m_fastest_lap
        details.vehicle_idx = fl.m_vehicle_idx
        details.lap_time = fl.m_lap_time
    elif event_code == "RTMT":
        details.vehicle_idx = ed.m_retirement.m_vehicle_idx
    elif event_code == "SPTP":
        st = ed.m_speed_trap
        details.vehicle_idx = st.m_vehicle_idx
        details.speed = st.m_speed
    elif event_code == "PENA":
        p = ed.m_penalty
        details.penalty_type = p.m_penalty_type
        details.infringement_type = p.m_infringement_type
        details.vehicle_idx = p.m_vehicle_idx
        details.other_vehicle_idx = p.m_other_vehicle_idx
        details.time = p.m_time
        details.lap_num = p.m_lap_num
        details.places_gained = p.m_places_gained
    elif event_code == "STLG":
        details.num_lights = ed.m_start_lights.m_num_lights
    elif event_code == "OVTK":
        details.overtaking_vehicle_idx = ed.m_overtake.m_overtakingVehicleIdx
        details.being_overtaken_vehicle_idx = ed.m_overtake.m_beingOvertakenVehicleIdx
    elif event_code == "SAFC":
        details.safety_car_type = ed.m_satefy_car.m_safetyCarType
        details.event_type = ed.m_satefy_car.m_eventType
    elif event_code == "COLL":
        details.vehicle_idx = ed.m_collision.m_vehicle1Idx
        details.other_vehicle_idx = ed.m_collision.m_vehicle2Idx

    return packets.PacketEventData(
        header=header,
        event_string_code=event_code,
        event_details=details,
    )


def _convert_participants(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    parts = []
    for i in range(min(ct_pkt.m_num_active_cars, 22)):
        p = ct_pkt.m_participants[i]
        name = p.m_name
        if isinstance(name, bytes):
            name = name.decode("utf-8", errors="replace").rstrip("\x00")
        parts.append(
            packets.ParticipantData(
                ai_controlled=p.m_ai_controlled,
                driver_id=p.m_driver_id,
                network_id=p.m_network_id,
                team_id=p.m_team_id,
                my_team=p.m_my_team,
                race_number=p.m_race_number,
                nationality=p.m_nationality,
                name=name,
                your_telemetry=p.m_your_telemetry,
                platform=p.m_platform,
            )
        )
    while len(parts) < 22:
        parts.append(packets.ParticipantData())
    return packets.PacketParticipantsData(
        header=header,
        num_active_cars=ct_pkt.m_num_active_cars,
        participants=parts,
    )


def _convert_car_setup(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    setups = []
    for i in range(22):
        s = ct_pkt.m_car_setups[i]
        setups.append(
            packets.CarSetup(
                front_wing=s.m_front_wing,
                rear_wing=s.m_rear_wing,
                on_throttle=s.m_on_throttle,
                off_throttle=s.m_off_throttle,
                front_camber=s.m_front_camber,
                rear_camber=s.m_rear_camber,
                front_toe=s.m_front_toe,
                rear_toe=s.m_rear_toe,
                front_suspension=s.m_front_suspension,
                rear_suspension=s.m_rear_suspension,
                front_anti_roll_bar=s.m_front_anti_roll_bar,
                rear_anti_roll_bar=s.m_rear_anti_roll_bar,
                front_suspension_height=s.m_front_suspension_height,
                rear_suspension_height=s.m_rear_suspension_height,
                brake_pressure=s.m_brake_pressure,
                brake_bias=s.m_brake_bias,
                engine_braking=s.m_engine_braking,
                front_left_tyre_pressure=s.m_front_left_tyre_pressure,
                front_right_tyre_pressure=s.m_front_right_tyre_pressure,
                rear_left_tyre_pressure=s.m_rear_left_tyre_pressure,
                rear_right_tyre_pressure=s.m_rear_right_tyre_pressure,
                ballast=s.m_ballast,
                fuel_load=s.m_fuel_load,
            )
        )
    return packets.PacketCarSetupData(header=header, car_setups=setups)


def _convert_car_telemetry(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    cars = []
    for i in range(22):
        c = ct_pkt.m_car_telemetry_data[i]
        cars.append(
            packets.CarTelemetry(
                speed=c.m_speed,
                throttle=c.m_throttle,
                steer=c.m_steer,
                brake=c.m_brake,
                clutch=c.m_clutch,
                gear=c.m_gear,
                engine_rpm=c.m_engine_rpm,
                drs=c.m_drs,
                rev_lights_percent=c.m_rev_lights_percent,
                rev_lights_bit_value=c.m_rev_lights_bit_value,
                brakes_temperature=list(c.m_brakes_temperature),
                tyres_surface_temperature=list(c.m_tyres_surface_temperature),
                tyres_inner_temperature=list(c.m_tyres_inner_temperature),
                engine_temperature=c.m_engine_temperature,
                tyres_pressure=list(c.m_tyres_pressure),
                surface_type=list(c.m_surface_type),
            )
        )
    return packets.PacketCarTelemetryData(
        header=header,
        car_telemetry_data=cars,
        mfd_panel_index=ct_pkt.m_mfd_panel_index,
        mfd_panel_index_secondary_player=ct_pkt.m_mfd_panel_index_secondary_player,
        suggested_gear=ct_pkt.m_suggested_gear,
    )


def _convert_car_status(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    cars = []
    for i in range(22):
        c = ct_pkt.m_car_status_data[i]
        cars.append(
            packets.CarStatus(
                traction_control=c.m_traction_control,
                anti_lock_brakes=c.m_anti_lock_brakes,
                fuel_mix=c.m_fuel_mix,
                front_brake_bias=c.m_front_brake_bias,
                pit_limiter_status=c.m_pit_limiter_status,
                fuel_in_tank=c.m_fuel_in_tank,
                fuel_capacity=c.m_fuel_capacity,
                fuel_remaining_laps=c.m_fuel_remaining_laps,
                max_rpm=c.m_max_rpm,
                idle_rpm=c.m_idle_rpm,
                max_gears=c.m_max_gears,
                drs_allowed=c.m_drs_allowed,
                drs_activation_distance=c.m_drs_activation_distance,
                actual_tyre_compound=c.m_actual_tyre_compound,
                visual_tyre_compound=c.m_visual_tyre_compound,
                tyres_age_laps=c.m_tyres_age_laps,
                vehicle_fia_flags=c.m_vehicle_fia_flags,
                engine_power_ice=c.m_engine_power_ice,
                engine_power_mguk=c.m_engine_power_mguk,
                ers_store_energy=c.m_ers_store_energy,
                ers_deploy_mode=c.m_ers_deploy_mode,
                ers_harvested_this_lap_mguk=c.m_ers_harvested_this_lap_mguk,
                ers_harvested_this_lap_mguh=c.m_ers_harvested_this_lap_mguh,
                ers_deployed_this_lap=c.m_ers_deployed_this_lap,
                network_paused=c.m_network_paused,
            )
        )
    return packets.PacketCarStatusData(header=header, car_status_data=cars)


def _convert_car_damage(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    cars = []
    for i in range(22):
        c = ct_pkt.m_car_damage_data[i]
        cars.append(
            packets.CarDamage(
                tyres_wear=list(c.m_tyres_wear),
                tyres_damage=list(c.m_tyres_damage),
                tyre_blisters=list(c.m_tyre_blisters),
                brakes_damage=list(c.m_brakes_damage),
                front_left_wing_damage=c.m_front_left_wing_damage,
                front_right_wing_damage=c.m_front_right_wing_damage,
                rear_wing_damage=c.m_rear_wing_damage,
                floor_damage=c.m_floor_damage,
                diffuser_damage=c.m_diffuser_damage,
                sidepod_damage=c.m_sidepod_damage,
                drs_fault=c.m_drs_fault,
                ers_fault=c.m_ers_fault,
                gear_box_damage=c.m_gear_box_damage,
                engine_damage=c.m_engine_damage,
                engine_mguh_wear=c.m_engine_mguhwear,
                engine_es_wear=c.m_engine_eswear,
                engine_ce_wear=c.m_engine_cewear,
                engine_ice_wear=c.m_engine_icewear,
                engine_mguk_wear=c.m_engine_mgukwear,
                engine_tc_wear=c.m_engine_tcwear,
                engine_blown=c.m_engine_blown,
                engine_seized=c.m_engine_seized,
            )
        )
    return packets.PacketCarDamageData(header=header, car_damage_data=cars)


def _convert_session_history(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    laps = []
    for i in range(min(ct_pkt.m_num_laps, 100)):
        l = ct_pkt.m_lap_history_data[i]
        laps.append(
            packets.LapHistoryData(
                lap_time_in_ms=l.m_lap_time_in_ms,
                sector1_time_in_ms=l.m_sector1_time_in_ms,
                sector1_time_minutes=l.m_sector1_time_in_minutes_part,
                sector2_time_in_ms=l.m_sector2_time_in_ms,
                sector2_time_minutes=l.m_sector2_time_in_minutes_part,
                sector3_time_in_ms=l.m_sector3_time_in_ms,
                sector3_time_minutes=l.m_sector3_time_in_minutes_part,
                lap_valid_bit_flags=l.m_lap_valid_bit_flags,
            )
        )
    stints = []
    for i in range(min(ct_pkt.m_num_tyre_stints, 8)):
        s = ct_pkt.m_tyre_stints_history_data[i]
        stints.append(
            packets.TyreStintHistoryData(
                end_lap=s.m_end_lap,
                tyre_actual_compound=s.m_tyre_actual_compound,
                tyre_visual_compound=s.m_tyre_visual_compound,
            )
        )
    return packets.PacketSessionHistoryData(
        header=header,
        car_idx=ct_pkt.m_car_idx,
        num_laps=ct_pkt.m_num_laps,
        num_tyre_stints=ct_pkt.m_num_tyre_stints,
        best_lap_time_lap_num=ct_pkt.m_best_lap_time_lap_num,
        best_sector1_lap_num=ct_pkt.m_best_sector1_lap_num,
        best_sector2_lap_num=ct_pkt.m_best_sector2_lap_num,
        best_sector3_lap_num=ct_pkt.m_best_sector3_lap_num,
        lap_history_data=laps,
        tyre_stint_history_data=stints,
    )


def _convert_tyre_sets(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    sets = []
    for i in range(20):
        t = ct_pkt.m_tyre_set_data[i]
        sets.append(
            packets.TyreSetData(
                actual_tyre_compound=t.m_actual_tyre_compound,
                visual_tyre_compound=t.m_visual_tyre_compound,
                wear=t.m_wear,
                available=t.m_available,
                recommended_session=t.m_recommanded_session,
                life_span=t.m_life_span,
                usable_life=t.m_usable_life,
                lap_delta_time=t.m_lap_delta_time,
                fitted=t.m_fitted,
            )
        )
    return packets.PacketTyreSetsData(
        header=header,
        car_idx=ct_pkt.m_car_idx,
        fitted_idx=ct_pkt.m_fitted_idx,
        tyre_set_data=sets,
    )


def _convert_motion_ex(ct_pkt):
    header = _convert_header(ct_pkt.m_header)
    return packets.PacketMotionExData(
        header=header,
        suspension_position=list(ct_pkt.m_suspension_position),
        suspension_velocity=list(ct_pkt.m_suspension_velocity),
        suspension_acceleration=list(ct_pkt.m_suspension_acceleration),
        wheel_speed=list(ct_pkt.m_wheel_speed),
        wheel_slip_ratio=list(ct_pkt.m_wheel_slip_ratio),
        wheel_slip_angle=list(ct_pkt.m_wheel_slip_angle),
        wheel_lat_force=list(ct_pkt.m_wheel_lat_force),
        wheel_long_force=list(ct_pkt.m_wheel_long_force),
        wheel_vert_force=list(ct_pkt.m_wheelVertForce),
        height_of_cog_above_ground=ct_pkt.m_height_of_cog_above_ground,
        front_aero_height=ct_pkt.m_front_aero_height,
        rear_aero_height=ct_pkt.m_rear_aero_height,
        front_roll_angle=ct_pkt.m_front_roll_angle,
        rear_roll_angle=ct_pkt.m_rear_roll_angle,
        chassis_yaw=ct_pkt.m_chassis_yaw,
        local_velocity_x=ct_pkt.m_local_velocity_x,
        local_velocity_y=ct_pkt.m_local_velocity_y,
        local_velocity_z=ct_pkt.m_local_velocity_z,
        angular_velocity_x=ct_pkt.m_angular_velocity_x,
        angular_velocity_y=ct_pkt.m_angular_velocity_y,
        angular_velocity_z=ct_pkt.m_angular_velocity_z,
        angular_acceleration_x=ct_pkt.m_angular_acceleration_x,
        angular_acceleration_y=ct_pkt.m_angular_acceleration_y,
        angular_acceleration_z=ct_pkt.m_angular_acceleration_z,
        front_wheels_angle=ct_pkt.m_front_wheels_angle,
        wheel_camber=list(ct_pkt.m_wheel_camber),
        wheel_camber_gain=list(ct_pkt.m_wheel_camber_gain),
    )


# ---------------------------------------------------------------------------
# Packet topic mapping & converter dispatch
# ---------------------------------------------------------------------------

_PACKET_TOPICS = {
    0: "packet_motion",
    1: "packet_session",
    2: "packet_lap_data",
    3: "packet_event",
    4: "packet_participants",
    5: "packet_car_setup",
    6: "packet_car_telemetry",
    7: "packet_car_status",
    10: "packet_car_damage",
    11: "packet_session_history",
    12: "packet_tyre_sets",
    13: "packet_motion_ex",
}

_CONVERTERS = {
    0: _convert_motion,
    1: _convert_session,
    2: _convert_lap_data,
    3: _convert_event,
    4: _convert_participants,
    5: _convert_car_setup,
    6: _convert_car_telemetry,
    7: _convert_car_status,
    10: _convert_car_damage,
    11: _convert_session_history,
    12: _convert_tyre_sets,
    13: _convert_motion_ex,
}


# ---------------------------------------------------------------------------
# Real UDP Parser
# ---------------------------------------------------------------------------


class RealTelemetryParser:
    """
    Listens for real F1 25 UDP telemetry packets on the network.
    Parses binary packets using ctypes and converts to Pydantic models,
    publishing on the same event bus topics as the mock parser.
    """

    def __init__(self, host: str = "0.0.0.0", port: int = 20777):
        self.host = host
        self.port = port
        self._is_running = False
        self._socket: Optional[socket.socket] = None
        self._last_packet_time: float = 0.0
        self._connected = False
        self._connection_timeout = 5.0

    @property
    def is_connected(self) -> bool:
        if self._last_packet_time == 0:
            return False
        return (time.time() - self._last_packet_time) < self._connection_timeout

    async def start(self):
        if _ct is None:
            logger.error(
                "Cannot start real parser: ctypes packet definitions not found. "
                "Ensure f1-25-telemetry-application/ is present in the project root."
            )
            await bus.publish(
                "telemetry_status",
                {
                    "mode": "real",
                    "status": "error",
                    "error": "Missing F1 25 parser definitions",
                    "host": self.host,
                    "port": self.port,
                },
            )
            return

        self._is_running = True

        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.bind((self.host, self.port))
            sock.setblocking(False)
            self._socket = sock
        except OSError as e:
            logger.error(f"Failed to bind UDP socket on {self.host}:{self.port}: {e}")
            await bus.publish(
                "telemetry_status",
                {
                    "mode": "real",
                    "status": "error",
                    "error": str(e),
                    "host": self.host,
                    "port": self.port,
                },
            )
            self._is_running = False
            return

        logger.info(f"Real Telemetry Parser listening on {self.host}:{self.port}")
        await bus.publish(
            "telemetry_status",
            {
                "mode": "real",
                "status": "listening",
                "host": self.host,
                "port": self.port,
            },
        )

        await self._listen_loop()

    def stop(self):
        self._is_running = False
        if self._socket:
            try:
                self._socket.close()
            except OSError:
                pass
            self._socket = None
        self._connected = False
        logger.info("Real Telemetry Parser stopped.")

    async def _listen_loop(self):
        loop = asyncio.get_running_loop()

        while self._is_running:
            try:
                data = await asyncio.wait_for(
                    loop.sock_recv(self._socket, 2048),
                    timeout=1.0,
                )
                self._last_packet_time = time.time()
                if not self._connected:
                    self._connected = True
                    logger.info("F1 25 game connected - receiving UDP packets")
                    await bus.publish(
                        "telemetry_status",
                        {
                            "mode": "real",
                            "status": "connected",
                            "host": self.host,
                            "port": self.port,
                        },
                    )
                await self._process_packet(data)

            except asyncio.TimeoutError:
                if self._connected and not self.is_connected:
                    self._connected = False
                    logger.warning("F1 25 game disconnected (no packets received)")
                    await bus.publish(
                        "telemetry_status",
                        {
                            "mode": "real",
                            "status": "disconnected",
                            "host": self.host,
                            "port": self.port,
                        },
                    )
            except asyncio.CancelledError:
                break
            except OSError:
                if self._is_running:
                    await asyncio.sleep(0.1)
            except Exception as e:
                if self._is_running:
                    logger.error(f"UDP listen error: {e}")
                    await asyncio.sleep(0.1)

    async def _process_packet(self, data: bytes):
        if len(data) < ctypes.sizeof(_ct.PacketHeader):
            return

        header = _ct.PacketHeader.from_buffer_copy(data)
        packet_id = header.m_packet_id

        packet_type = _ct.HEADER_FIELD_TO_PACKET_TYPE.get(packet_id)
        if not packet_type:
            return

        try:
            ct_packet = packet_type.from_buffer_copy(data)
        except ValueError:
            return

        topic = _PACKET_TOPICS.get(packet_id)
        converter = _CONVERTERS.get(packet_id)
        if topic and converter:
            try:
                pydantic_model = converter(ct_packet)
                await bus.publish(topic, pydantic_model)
            except Exception as e:
                logger.debug(f"Conversion error for packet {packet_id}: {e}")
