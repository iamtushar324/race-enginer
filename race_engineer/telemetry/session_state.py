"""
Unified in-memory snapshot of all latest telemetry data.
Single source of truth for all packet types - subscribers read from here.
"""

import logging
from typing import Dict, List, Optional

from race_engineer.core.event_bus import bus
from race_engineer.telemetry.models import TelemetryTick
from race_engineer.telemetry.packets import (
    PacketMotionData,
    PacketSessionData,
    PacketLapData,
    PacketEventData,
    PacketParticipantsData,
    PacketCarSetupData,
    PacketCarTelemetryData,
    PacketCarStatusData,
    PacketFinalClassificationData,
    PacketLobbyInfoData,
    PacketCarDamageData,
    PacketSessionHistoryData,
    PacketTyreSetsData,
    PacketMotionExData,
    PacketTimeTrialData,
    PacketLapPositions,
)

logger = logging.getLogger(__name__)


class SessionState:
    """Holds the latest data from every packet type as a single source of truth."""

    def __init__(self):
        self.motion: Optional[PacketMotionData] = None
        self.session: Optional[PacketSessionData] = None
        self.lap_data: Optional[PacketLapData] = None
        self.event: Optional[PacketEventData] = None
        self.participants: Optional[PacketParticipantsData] = None
        self.car_setups: Optional[PacketCarSetupData] = None
        self.car_telemetry: Optional[PacketCarTelemetryData] = None
        self.car_status: Optional[PacketCarStatusData] = None
        self.final_classification: Optional[PacketFinalClassificationData] = None
        self.lobby_info: Optional[PacketLobbyInfoData] = None
        self.car_damage: Optional[PacketCarDamageData] = None
        self.session_history: Dict[int, PacketSessionHistoryData] = {}
        self.tyre_sets: Optional[PacketTyreSetsData] = None
        self.motion_ex: Optional[PacketMotionExData] = None
        self.time_trial: Optional[PacketTimeTrialData] = None
        self.lap_positions: Optional[PacketLapPositions] = None
        self.events_log: List[PacketEventData] = []

        # Subscribe to all packet bus topics
        bus.subscribe("packet_motion", self._on_motion)
        bus.subscribe("packet_session", self._on_session)
        bus.subscribe("packet_lap_data", self._on_lap_data)
        bus.subscribe("packet_event", self._on_event)
        bus.subscribe("packet_participants", self._on_participants)
        bus.subscribe("packet_car_setup", self._on_car_setup)
        bus.subscribe("packet_car_telemetry", self._on_car_telemetry)
        bus.subscribe("packet_car_status", self._on_car_status)
        bus.subscribe("packet_final_classification", self._on_final_classification)
        bus.subscribe("packet_lobby_info", self._on_lobby_info)
        bus.subscribe("packet_car_damage", self._on_car_damage)
        bus.subscribe("packet_session_history", self._on_session_history)
        bus.subscribe("packet_tyre_sets", self._on_tyre_sets)
        bus.subscribe("packet_motion_ex", self._on_motion_ex)
        bus.subscribe("packet_time_trial", self._on_time_trial)
        bus.subscribe("packet_lap_positions", self._on_lap_positions)

        logger.info("SessionState initialized - subscribing to all packet topics.")

    # --- Event handlers ---

    async def _on_motion(self, data: PacketMotionData):
        self.motion = data

    async def _on_session(self, data: PacketSessionData):
        self.session = data

    async def _on_lap_data(self, data: PacketLapData):
        self.lap_data = data

    async def _on_event(self, data: PacketEventData):
        self.event = data
        self.events_log.append(data)
        # Cap events log at 200 entries
        if len(self.events_log) > 200:
            self.events_log = self.events_log[-200:]

    async def _on_participants(self, data: PacketParticipantsData):
        self.participants = data

    async def _on_car_setup(self, data: PacketCarSetupData):
        self.car_setups = data

    async def _on_car_telemetry(self, data: PacketCarTelemetryData):
        self.car_telemetry = data
        # Re-publish backward-compatible telemetry_tick
        await self._publish_telemetry_tick()

    async def _on_car_status(self, data: PacketCarStatusData):
        self.car_status = data

    async def _on_final_classification(self, data: PacketFinalClassificationData):
        self.final_classification = data

    async def _on_lobby_info(self, data: PacketLobbyInfoData):
        self.lobby_info = data

    async def _on_car_damage(self, data: PacketCarDamageData):
        self.car_damage = data

    async def _on_session_history(self, data: PacketSessionHistoryData):
        self.session_history[data.car_idx] = data

    async def _on_tyre_sets(self, data: PacketTyreSetsData):
        self.tyre_sets = data

    async def _on_motion_ex(self, data: PacketMotionExData):
        self.motion_ex = data

    async def _on_time_trial(self, data: PacketTimeTrialData):
        self.time_trial = data

    async def _on_lap_positions(self, data: PacketLapPositions):
        self.lap_positions = data

    # --- Backward compatibility ---

    async def _publish_telemetry_tick(self):
        """
        Constructs a legacy TelemetryTick from the new packet data and publishes it.
        This keeps all existing subscribers (analyzer, advisor, UI, store) working.
        """
        if not self.car_telemetry or not self.lap_data:
            return

        idx = self.car_telemetry.header.player_car_index
        if idx >= len(self.car_telemetry.car_telemetry_data):
            return

        ct = self.car_telemetry.car_telemetry_data[idx]
        lap = (
            self.lap_data.car_lap_data[idx]
            if idx < len(self.lap_data.car_lap_data)
            else None
        )

        # Get tire wear from damage packet if available
        wear_fl, wear_fr, wear_rl, wear_rr = 0.0, 0.0, 0.0, 0.0
        if self.car_damage and idx < len(self.car_damage.car_damage_data):
            dmg = self.car_damage.car_damage_data[idx]
            wear_rl, wear_rr, wear_fl, wear_fr = dmg.tyres_wear  # [RL, RR, FL, FR]

        # Track position as 0.0-1.0
        track_length = 5412  # default
        if self.session:
            track_length = self.session.track_length
        track_pos = 0.0
        if lap and track_length > 0:
            track_pos = max(0.0, min(1.0, lap.lap_distance / track_length))

        # Sector (API uses 0-based, TelemetryTick uses 1-based)
        sector = 1
        if lap:
            sector = lap.sector + 1

        current_lap = lap.current_lap_num if lap else 1

        tick = TelemetryTick(
            speed=float(ct.speed),
            gear=ct.gear,
            throttle=ct.throttle,
            brake=ct.brake,
            steering=ct.steer,
            engine_rpm=ct.engine_rpm,
            tire_wear_fl=wear_fl,
            tire_wear_fr=wear_fr,
            tire_wear_rl=wear_rl,
            tire_wear_rr=wear_rr,
            lap=current_lap,
            track_position=track_pos,
            sector=sector,
        )
        await bus.publish("telemetry_tick", tick)

    def to_telemetry_tick(self) -> Optional[TelemetryTick]:
        """Synchronous version - constructs TelemetryTick from current state."""
        if not self.car_telemetry or not self.lap_data:
            return None

        idx = self.car_telemetry.header.player_car_index
        if idx >= len(self.car_telemetry.car_telemetry_data):
            return None

        ct = self.car_telemetry.car_telemetry_data[idx]
        lap = (
            self.lap_data.car_lap_data[idx]
            if idx < len(self.lap_data.car_lap_data)
            else None
        )

        wear_fl, wear_fr, wear_rl, wear_rr = 0.0, 0.0, 0.0, 0.0
        if self.car_damage and idx < len(self.car_damage.car_damage_data):
            dmg = self.car_damage.car_damage_data[idx]
            wear_rl, wear_rr, wear_fl, wear_fr = dmg.tyres_wear

        track_length = self.session.track_length if self.session else 5412
        track_pos = 0.0
        if lap and track_length > 0:
            track_pos = max(0.0, min(1.0, lap.lap_distance / track_length))

        sector = (lap.sector + 1) if lap else 1
        current_lap = lap.current_lap_num if lap else 1

        return TelemetryTick(
            speed=float(ct.speed),
            gear=ct.gear,
            throttle=ct.throttle,
            brake=ct.brake,
            steering=ct.steer,
            engine_rpm=ct.engine_rpm,
            tire_wear_fl=wear_fl,
            tire_wear_fr=wear_fr,
            tire_wear_rl=wear_rl,
            tire_wear_rr=wear_rr,
            lap=current_lap,
            track_position=track_pos,
            sector=sector,
        )
