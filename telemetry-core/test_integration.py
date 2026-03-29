"""Integration test for telemetry-core using proper F1 25 binary packets.

Requires a running server: TELEMETRY_MODE=real go run cmd/server/main.go

This script:
  1. Sends valid binary F1 25 packets over UDP (all 8 handled types)
  2. Polls the REST API for telemetry and insights
  3. Verifies DuckDB row counts via POST /api/query
"""

import socket
import struct
import time
import json
import sys
import requests

UDP_IP = "127.0.0.1"
UDP_PORT = 20777
API_URL = "http://127.0.0.1:8081"

# ---------------------------------------------------------------------------
# Binary packet builders — little-endian, matching Go struct layouts exactly.
# ---------------------------------------------------------------------------

# PacketHeader: 29 bytes
# H=uint16(PacketFormat), B*5(GameYear..PacketID), Q=uint64(SessionUID),
# f=float32(SessionTime), I*2(FrameID, OverallFrameID), B*2(PlayerCarIdx, Secondary)
HEADER_FMT = "<HBBBBBQfIIBB"
HEADER_SIZE = struct.calcsize(HEADER_FMT)  # 29


def make_header(packet_id: int, player_idx: int = 0) -> bytes:
    return struct.pack(
        HEADER_FMT,
        2025,        # PacketFormat
        25,          # GameYear
        1, 0, 1,     # GameMajorVersion, GameMinorVersion, PacketVersion
        packet_id,   # PacketID
        999,         # SessionUID
        42.0,        # SessionTime
        1,           # FrameIdentifier
        1,           # OverallFrameID
        player_idx,  # PlayerCarIndex
        0,           # SecondaryPlayerCarIndex
    )


def build_motion_packet(player_idx: int = 0, x: float = 100.0, y: float = 200.0, z: float = 300.0) -> bytes:
    """Packet ID 0 — 1349 bytes. CarMotionData = 60 bytes × 22 cars."""
    header = make_header(0, player_idx)
    # CarMotionData: 3×f(pos) + 3×f(vel) + 6×h(dirs) + 3×f(g) + 3×f(orient) = 60 bytes
    CAR_FMT = "<3f3f6h3f3f"
    car_data = struct.pack(CAR_FMT, x, y, z, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1.5, 0.3, 1.0, 0.1, 0.02, 0.03)
    empty_car = b"\x00" * 60
    cars = b""
    for i in range(22):
        cars += car_data if i == player_idx else empty_car
    pkt = header + cars
    assert len(pkt) == 1349, f"motion packet size {len(pkt)} != 1349"
    return pkt


def build_session_packet(weather: int = 0, track_temp: int = 30, total_laps: int = 50, safety_car: int = 0) -> bytes:
    """Packet ID 1 — 753 bytes."""
    header = make_header(1, 0)
    # Fill remaining 724 bytes. Key fields at known offsets after header:
    # weather(B), track_temp(b), air_temp(b), total_laps(B), track_length(H),
    # session_type(B), track_id(b), formula(B), session_time_left(H), session_duration(H),
    # ... then marshal zones, weather forecasts, and many more fields.
    body = bytearray(753 - HEADER_SIZE)
    body[0] = weather & 0xFF                          # weather
    body[1] = track_temp & 0xFF                       # track_temperature (int8)
    body[2] = (track_temp - 5) & 0xFF                 # air_temperature (int8)
    body[3] = total_laps & 0xFF                       # total_laps
    struct.pack_into("<H", body, 4, 5303)             # track_length
    body[6] = 10                                      # session_type (Race)
    body[7] = 0                                       # track_id
    # safety_car_status is after marshal zones and other fields.
    # Offset: formula(1) + session_time_left(2) + session_duration(2) + pit_speed_limit(1)
    # + game_paused(1) + is_spectating(1) + spectator_car_index(1) + sli_pro(1)
    # + num_marshal_zones(1) + marshal_zones(21*5=105) = 8+1+1+105 = 117
    # So safety_car_status is at body[8 + 1+2+2+1+1+1+1+1+1+105] = body[8+116] = body[124]
    body[124] = safety_car & 0xFF                     # safety_car_status
    return header + bytes(body)


def build_lap_data_packet(player_idx: int = 0, lap_num: int = 5, position: int = 3, sector: int = 1) -> bytes:
    """Packet ID 2 — 1285 bytes. LapData = 57 bytes × 22 cars + 2 trailing."""
    header = make_header(2, player_idx)
    # LapData struct: I I H B H B H B H B f f f B*14 B H H B f B = 57 bytes
    LAP_FMT = "<IIHBHBHBHBfff14BHHB fB"
    car_data = struct.pack(
        LAP_FMT,
        90000, 45000,    # last_lap_time, current_lap_time
        30000, 0,        # sector1_time_ms, sector1_minutes
        30000, 0,        # sector2_time_ms, sector2_minutes
        0, 0,            # delta_front_ms, delta_front_minutes
        0, 0,            # delta_leader_ms, delta_leader_minutes
        1000.0, 5000.0, 0.0,  # lap_distance, total_distance, safety_car_delta
        position, lap_num, 0, 0,  # position, lap_num, pit_status, num_pit_stops
        sector, 0, 0, 0, 0, 0, 0, position, 1, 2, 0,  # sector..result_status
        0, 0, 0,         # pit_lane_timer, pit_lane_time, pit_stop_timer
        0,               # pit_stop_should_serve_pen (uint8, was missing)
        0.0,             # speed_trap_fastest_speed
        0,               # speed_trap_fastest_lap
    )
    empty_car = b"\x00" * 57
    cars = b""
    for i in range(22):
        cars += car_data if i == player_idx else empty_car
    trailing = b"\x00\x00"  # TimeTrialPBCarIdx + TimeTrialRivalCarIdx
    pkt = header + cars + trailing
    assert len(pkt) == 1285, f"lap_data packet size {len(pkt)} != 1285"
    return pkt


def build_event_packet(code: str = "FTLP") -> bytes:
    """Packet ID 3 — 45 bytes."""
    header = make_header(3, 0)
    event_code = code.encode("ascii")[:4].ljust(4, b"\x00")
    event_details = bytearray(12)
    event_details[0] = 5  # vehicle_idx
    return header + event_code + bytes(event_details)


def build_car_telemetry_packet(player_idx: int = 0, speed: int = 250, throttle: float = 0.8, brake: float = 0.1) -> bytes:
    """Packet ID 6 — 1352 bytes. CarTelemetryData = 60 bytes × 22 + 3 trailing."""
    header = make_header(6, player_idx)
    # CarTelemetryData: H f f f B b H B B H 4H 4B 4B H 4f 4B = 60 bytes
    CAR_FMT = "<HfffBbHBBH4H4B4BH4f4B"
    car_data = struct.pack(
        CAR_FMT,
        speed, throttle, 0.1, brake,  # speed, throttle, steer, brake
        50, 5, 10000, 0, 80, 0,       # clutch, gear, rpm, drs, rev_lights_pct, rev_lights_bit
        500, 510, 520, 530,            # brakes_temp[4]
        95, 96, 97, 98,               # tyres_surface_temp[4]
        100, 101, 102, 103,           # tyres_inner_temp[4]
        110,                           # engine_temperature
        23.0, 23.1, 23.2, 23.3,      # tyres_pressure[4]
        0, 0, 0, 0,                   # surface_type[4]
    )
    empty_car = b"\x00" * 60
    cars = b""
    for i in range(22):
        cars += car_data if i == player_idx else empty_car
    trailing = struct.pack("<BBb", 0, 0, 6)  # MFDPanel, MFDSecondary, SuggestedGear
    pkt = header + cars + trailing
    assert len(pkt) == 1352, f"telemetry packet size {len(pkt)} != 1352"
    return pkt


def build_car_status_packet(player_idx: int = 0, fuel: float = 50.0, ers: float = 2000000.0) -> bytes:
    """Packet ID 7 — 1239 bytes. CarStatusData = 55 bytes × 22."""
    header = make_header(7, player_idx)
    # CarStatusData: 5B 3f 2H 2B H 3B b 3f B 3f B = 55 bytes
    CAR_FMT = "<5B3f2H2BH3BbfffBfffB"
    car_data = struct.pack(
        CAR_FMT,
        0, 0, 2, 50, 0,               # traction, abs, fuel_mix, front_brake_bias, pit_limiter
        fuel, 110.0, 15.5,            # fuel_in_tank, fuel_capacity, fuel_remaining_laps
        15000, 3000,                   # max_rpm, idle_rpm
        8, 1,                          # max_gears, drs_allowed
        100,                           # drs_activation_distance
        16, 16, 5,                     # actual_compound, visual_compound, tyres_age
        0,                             # vehicle_fia_flags (int8)
        750000.0, 120000.0, ers,      # power_ice, power_mguk, ers_store
        1,                             # ers_deploy_mode
        100000.0, 200000.0, 150000.0, # ers_mguk, ers_mguh, ers_deployed
        0,                             # network_paused
    )
    empty_car = b"\x00" * 55
    cars = b""
    for i in range(22):
        cars += car_data if i == player_idx else empty_car
    pkt = header + cars
    assert len(pkt) == 1239, f"car_status packet size {len(pkt)} != 1239"
    return pkt


def build_car_damage_packet(player_idx: int = 0, tyres_wear: tuple = (10.0, 11.0, 12.0, 13.0)) -> bytes:
    """Packet ID 10 — 1041 bytes. CarDamageData = 46 bytes × 22."""
    header = make_header(10, player_idx)
    # CarDamageData: 4f 4B 4B 4B 18B = 46 bytes
    CAR_FMT = "<4f4B4B4B18B"
    car_data = struct.pack(
        CAR_FMT,
        *tyres_wear,                   # tyres_wear[4]
        5, 6, 7, 8,                   # tyres_damage[4]
        2, 2, 2, 2,                   # brakes_damage[4]
        0, 0, 0, 0,                   # tyre_blisters[4]
        10, 12, 3, 1, 0, 0,          # fl_wing, fr_wing, rear_wing, floor, diffuser, sidepod
        0, 0,                          # drs_fault, ers_fault
        0, 5,                          # gearbox, engine
        0, 0, 0, 0, 0, 0,            # mguh/es/ce/ice/mguk/tc wear
        0, 0,                          # engine_blown, engine_seized
    )
    empty_car = b"\x00" * 46
    cars = b""
    for i in range(22):
        cars += car_data if i == player_idx else empty_car
    pkt = header + cars
    assert len(pkt) == 1041, f"car_damage packet size {len(pkt)} != 1041"
    return pkt


def build_session_history_packet(car_idx: int = 0, num_laps: int = 3, lap_times: tuple = (90000, 91000, 92000)) -> bytes:
    """Packet ID 11 — 1460 bytes."""
    header = make_header(11, 0)
    # 7 bytes of metadata + 100 * LapHistoryData(14) + 8 * TyreStintHistory(3) = 1431
    body = bytearray(1460 - HEADER_SIZE)
    body[0] = car_idx                  # car_idx
    body[1] = num_laps                 # num_laps
    body[2] = 1                        # num_tyre_stints
    body[3] = 1                        # best_lap_time_lap_num
    # LapHistoryData starts at offset 7, each 14 bytes:
    # I(lap_time) H(s1) B(s1_min) H(s2) B(s2_min) H(s3) B(s3_min) B(valid)
    LAP_FMT = "<IHBHBHBB"
    for i in range(min(num_laps, len(lap_times))):
        offset = 7 + i * 14
        s = lap_times[i] // 3
        struct.pack_into(LAP_FMT, body, offset,
                         lap_times[i], s, 0, s, 0, s, 0, 0x01)
    return header + bytes(body)


# ---------------------------------------------------------------------------
# Test runner
# ---------------------------------------------------------------------------

def send_packets(num: int = 60, rate_hz: int = 60):
    """Send valid binary packets for all 8 handled types."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    print(f"Sending {num} rounds of 8 packet types to {UDP_IP}:{UDP_PORT}")

    for i in range(num):
        sock.sendto(build_motion_packet(x=float(100 + i)), (UDP_IP, UDP_PORT))
        sock.sendto(build_session_packet(), (UDP_IP, UDP_PORT))
        sock.sendto(build_lap_data_packet(lap_num=i % 50 + 1), (UDP_IP, UDP_PORT))
        sock.sendto(build_event_packet("FTLP"), (UDP_IP, UDP_PORT))
        sock.sendto(build_car_telemetry_packet(speed=250 + i % 50), (UDP_IP, UDP_PORT))
        sock.sendto(build_car_status_packet(), (UDP_IP, UDP_PORT))
        sock.sendto(build_car_damage_packet(), (UDP_IP, UDP_PORT))
        sock.sendto(build_session_history_packet(), (UDP_IP, UDP_PORT))
        time.sleep(1 / rate_hz)

    sock.close()
    print(f"Sent {num * 8} total packets.")


def poll_api(duration_s: int = 10):
    """Poll API endpoints and verify data is present."""
    print(f"\nPolling API at {API_URL} for {duration_s}s...")
    end_time = time.time() + duration_s
    saw_telemetry = False
    saw_insight = False

    while time.time() < end_time:
        try:
            res = requests.get(f"{API_URL}/api/telemetry/latest", timeout=2)
            if res.status_code == 200:
                data = res.json()
                speed = data.get("speed", 0)
                if speed > 0:
                    saw_telemetry = True
                    print(f"  Telemetry OK — speed={speed}")

            res = requests.get(f"{API_URL}/api/insights/next", timeout=2)
            if res.status_code == 200 and res.text.strip():
                saw_insight = True
                print(f"  Insight: {res.json().get('message', '')[:60]}")

        except Exception as e:
            print(f"  API error: {e}")

        time.sleep(2)

    return saw_telemetry, saw_insight


def verify_row_counts():
    """Query DuckDB row counts via POST /api/query."""
    print("\nDuckDB row counts:")
    tables = [
        "telemetry", "car_telemetry_ext", "session_data", "lap_data",
        "motion_data", "car_status", "car_damage", "race_events",
    ]
    all_ok = True
    for tbl in tables:
        try:
            res = requests.post(
                f"{API_URL}/api/query",
                json={"sql": f"SELECT count(*) as n FROM {tbl}"},
                timeout=5,
            )
            if res.status_code == 200:
                rows = res.json()
                n = rows[0]["n"] if rows else 0
                status = "OK" if n > 0 else "EMPTY"
                if n == 0:
                    all_ok = False
                print(f"  {tbl:25s} {n:>6} rows  [{status}]")
            else:
                print(f"  {tbl:25s} error {res.status_code}")
                all_ok = False
        except Exception as e:
            print(f"  {tbl:25s} error: {e}")
            all_ok = False

    return all_ok


def main():
    print("=" * 60)
    print("F1 25 Telemetry Integration Test")
    print("=" * 60)

    # Phase 1: Send packets
    send_packets(num=60, rate_hz=60)

    # Phase 2: Wait for flush
    print("\nWaiting 3s for server to flush buffers...")
    time.sleep(3)

    # Phase 3: Poll API
    saw_telemetry, _ = poll_api(duration_s=8)

    # Phase 4: Verify DuckDB
    tables_ok = verify_row_counts()

    # Summary
    print("\n" + "=" * 60)
    print("RESULTS:")
    print(f"  Telemetry API responded: {'PASS' if saw_telemetry else 'FAIL'}")
    print(f"  DuckDB tables populated: {'PASS' if tables_ok else 'FAIL'}")
    print("=" * 60)

    if not saw_telemetry or not tables_ok:
        sys.exit(1)


if __name__ == "__main__":
    main()
