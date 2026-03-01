"""
F1 25 Telemetry Enumerations and Lookup Dictionaries.
Reference: f1-25-telemetry-application/src/packet_processing/dictionnaries.py
"""

from enum import IntEnum


class Weather(IntEnum):
    CLEAR = 0
    LIGHT_CLOUD = 1
    OVERCAST = 2
    LIGHT_RAIN = 3
    HEAVY_RAIN = 4
    STORM = 5


class SessionType(IntEnum):
    UNKNOWN = 0
    P1 = 1
    P2 = 2
    P3 = 3
    SHORT_P = 4
    Q1 = 5
    Q2 = 6
    Q3 = 7
    SHORT_Q = 8
    OSQ = 9
    R = 10
    R2 = 11
    R3 = 12
    TIME_TRIAL = 13


class TrackId(IntEnum):
    MELBOURNE = 0
    PAUL_RICARD = 1
    SHANGHAI = 2
    SAKHIR = 3
    CATALUNYA = 4
    MONACO = 5
    MONTREAL = 6
    SILVERSTONE = 7
    HOCKENHEIM = 8
    HUNGARORING = 9
    SPA = 10
    MONZA = 11
    SINGAPORE = 12
    SUZUKA = 13
    ABU_DHABI = 14
    TEXAS = 15
    BRAZIL = 16
    AUSTRIA = 17
    SOCHI = 18
    MEXICO = 19
    BAKU = 20
    SAKHIR_SHORT = 21
    SILVERSTONE_SHORT = 22
    TEXAS_SHORT = 23
    SUZUKA_SHORT = 24
    HANOI = 25
    ZANDVOORT = 26
    IMOLA = 27
    PORTIMAO = 28
    JEDDAH = 29
    MIAMI = 30
    LAS_VEGAS = 31
    LOSAIL = 32


class TyreCompound(IntEnum):
    C5 = 16
    C4 = 17
    C3 = 18
    C2 = 19
    C1 = 20
    INTER = 7
    WET = 8


class TyreCompoundVisual(IntEnum):
    SOFT = 16
    MEDIUM = 17
    HARD = 18
    INTER = 7
    WET = 8


class FuelMix(IntEnum):
    LEAN = 0
    STANDARD = 1
    RICH = 2
    MAX = 3


class ERSDeployMode(IntEnum):
    NONE = 0
    MEDIUM = 1
    HOTLAP = 2
    OVERTAKE = 3


class SafetyCarStatus(IntEnum):
    NONE = 0
    FULL = 1
    VIRTUAL = 2
    FORMATION_LAP = 3


class FIAFlag(IntEnum):
    INVALID = -1
    NONE = 0
    GREEN = 1
    BLUE = 2
    YELLOW = 3
    RED = 4


class DriverStatus(IntEnum):
    GARAGE = 0
    FLYING_LAP = 1
    IN_LAP = 2
    OUT_LAP = 3
    ON_TRACK = 4


class ResultStatus(IntEnum):
    INVALID = 0
    INACTIVE = 1
    ACTIVE = 2
    FINISHED = 3
    DNF = 4
    DISQUALIFIED = 5
    NOT_CLASSIFIED = 6
    RETIRED = 7


class PitStatus(IntEnum):
    NONE = 0
    PITTING = 1
    IN_PIT_AREA = 2


class EventCode:
    """F1 event string codes (4-char strings, not IntEnum)."""

    START_LIGHTS = "STLG"
    LIGHTS_OUT = "LGOT"
    RETIREMENT = "RTMT"
    FASTEST_LAP = "FTLP"
    DRS_DISABLED = "DRSD"
    DRS_ENABLED = "DRSE"
    CHEQUERED_FLAG = "CHQF"
    PENALTY = "PENA"
    SPEED_TRAP = "SPTP"
    TEAMMATE_IN_PITS = "TMPT"
    OVERTAKE = "OVTK"
    SAFETY_CAR = "SAFC"
    COLLISION = "COLL"


# --- Lookup Dictionaries ---

TEAM_NAMES = {
    -1: "Unknown",
    0: "Mercedes",
    1: "Ferrari",
    2: "Red Bull Racing",
    3: "Williams",
    4: "Aston Martin",
    5: "Alpine",
    6: "AlphaTauri",
    7: "Haas",
    8: "McLaren",
    9: "Alfa Romeo",
    41: "Multi",
    104: "Custom Team",
    255: "Custom Team",
}

TRACK_NAMES = {
    0: "Melbourne",
    1: "Paul Ricard",
    2: "Shanghai",
    3: "Sakhir",
    4: "Catalunya",
    5: "Monaco",
    6: "Montreal",
    7: "Silverstone",
    8: "Hockenheim",
    9: "Hungaroring",
    10: "Spa-Francorchamps",
    11: "Monza",
    12: "Singapore",
    13: "Suzuka",
    14: "Abu Dhabi",
    15: "Austin",
    16: "Interlagos",
    17: "Red Bull Ring",
    18: "Sochi",
    19: "Mexico City",
    20: "Baku",
    21: "Sakhir Short",
    22: "Silverstone Short",
    23: "Austin Short",
    24: "Suzuka Short",
    25: "Hanoi",
    26: "Zandvoort",
    27: "Imola",
    28: "Portimao",
    29: "Jeddah",
    30: "Miami",
    31: "Las Vegas",
    32: "Losail",
}

NATIONALITY_NAMES = {
    1: "American",
    2: "Argentinean",
    3: "Australian",
    4: "Austrian",
    5: "Azerbaijani",
    6: "Bahraini",
    7: "Belgian",
    8: "Bolivian",
    9: "Brazilian",
    10: "British",
    11: "Bulgarian",
    12: "Cameroonian",
    13: "Canadian",
    14: "Chilean",
    15: "Chinese",
    16: "Colombian",
    17: "Costa Rican",
    18: "Croatian",
    19: "Cypriot",
    20: "Czech",
    21: "Danish",
    22: "Dutch",
    23: "Ecuadorian",
    24: "English",
    25: "Emirian",
    26: "Estonian",
    27: "Finnish",
    28: "French",
    29: "German",
    30: "Ghanaian",
    31: "Greek",
    32: "Guatemalan",
    33: "Honduran",
    34: "Hong Konger",
    35: "Hungarian",
    36: "Icelander",
    37: "Indian",
    38: "Indonesian",
    39: "Irish",
    40: "Israeli",
    41: "Italian",
    42: "Jamaican",
    43: "Japanese",
    44: "Jordanian",
    45: "Kuwaiti",
    46: "Latvian",
    47: "Lebanese",
    48: "Lithuanian",
    49: "Luxembourger",
    50: "Malaysian",
    51: "Maltese",
    52: "Mexican",
    53: "Monegasque",
    54: "New Zealander",
    55: "Nicaraguan",
    56: "Northern Irish",
    57: "Norwegian",
    58: "Omani",
    59: "Pakistani",
    60: "Panamanian",
    61: "Paraguayan",
    62: "Peruvian",
    63: "Polish",
    64: "Portuguese",
    65: "Qatari",
    66: "Romanian",
    67: "Russian",
    68: "Salvadoran",
    69: "Saudi",
    70: "Scottish",
    71: "Serbian",
    72: "Singaporean",
    73: "Slovakian",
    74: "Slovenian",
    75: "South Korean",
    76: "South African",
    77: "Spanish",
    78: "Swedish",
    79: "Swiss",
    80: "Thai",
    81: "Turkish",
    82: "Uruguayan",
    83: "Ukrainian",
    84: "Venezuelan",
    85: "Barbadian",
    86: "Welsh",
    87: "Vietnamese",
}

WEATHER_NAMES = {
    0: "Clear",
    1: "Light Cloud",
    2: "Overcast",
    3: "Light Rain",
    4: "Heavy Rain",
    5: "Storm",
}

FUEL_MIX_NAMES = {
    0: "Lean",
    1: "Standard",
    2: "Rich",
    3: "Max",
}

ERS_MODE_NAMES = {
    0: "None",
    1: "Medium",
    2: "Hotlap",
    3: "Overtake",
    -1: "Private",
}

SAFETY_CAR_NAMES = {
    0: "None",
    1: "Safety Car",
    2: "Virtual SC",
    3: "Formation Lap",
    4: "None",
}

SESSION_TYPE_NAMES = {
    0: "Unknown",
    1: "P1",
    2: "P2",
    3: "P3",
    4: "Short Practice",
    5: "Q1",
    6: "Q2",
    7: "Q3",
    8: "Short Qualifying",
    9: "One-Shot Qualifying",
    10: "Race",
    11: "Race 2",
    12: "Race 3",
    13: "Time Trial",
}

TYRE_COMPOUND_NAMES = {
    16: "Soft",
    17: "Medium",
    18: "Hard",
    7: "Intermediate",
    8: "Wet",
}

TYRE_COMPOUND_SHORT = {
    16: "S",
    17: "M",
    18: "H",
    7: "I",
    8: "W",
}

PACKET_NAMES = {
    0: "motion",
    1: "session",
    2: "lap_data",
    3: "event",
    4: "participants",
    5: "car_setup",
    6: "car_telemetry",
    7: "car_status",
    8: "final_classification",
    9: "lobby_info",
    10: "car_damage",
    11: "session_history",
    12: "tyre_sets",
    13: "motion_ex",
    14: "time_trial",
    15: "lap_positions",
}
