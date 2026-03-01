import asyncio
import pytest
from race_engineer.core.event_bus import bus
from race_engineer.feedback.analyzer import PerformanceAnalyzer
from race_engineer.telemetry.models import TelemetryTick, DrivingInsight


@pytest.fixture
def analyzer():
    # Instantiating sets up the subscriptions
    return PerformanceAnalyzer()


@pytest.mark.asyncio
async def test_analyzer_detects_new_lap(analyzer):
    insights = []

    async def insight_handler(data: DrivingInsight):
        insights.append(data)

    bus.subscribe("driving_insight", insight_handler)

    tick = TelemetryTick(
        speed=100.0,
        gear=2,
        throttle=1.0,
        brake=0.0,
        steering=0.0,
        engine_rpm=5000,
        tire_wear_fl=5.0,
        tire_wear_fr=5.0,
        tire_wear_rl=5.0,
        tire_wear_rr=5.0,
        lap=2,  # Triggers new lap
        track_position=0.0,
        sector=1,
    )

    await bus.publish("telemetry_tick", tick)
    await asyncio.sleep(0.01)

    assert len(insights) > 0
    assert any("Lap 2" in i.message for i in insights)


@pytest.mark.asyncio
async def test_analyzer_detects_high_tire_wear(analyzer):
    insights = []

    async def insight_handler(data: DrivingInsight):
        insights.append(data)

    bus.subscribe("driving_insight", insight_handler)

    tick = TelemetryTick(
        speed=100.0,
        gear=2,
        throttle=1.0,
        brake=0.0,
        steering=0.0,
        engine_rpm=5000,
        tire_wear_fl=65.0,  # Triggers tire wear warning
        tire_wear_fr=5.0,
        tire_wear_rl=5.0,
        tire_wear_rr=5.0,
        lap=2,
        track_position=0.0,
        sector=1,
    )

    # Must wait slightly for async handlers to finish
    await bus.publish("telemetry_tick", tick)
    await asyncio.sleep(0.01)

    assert any(
        i.type == "strategy" and "Tires are heavily worn" in i.message for i in insights
    )
