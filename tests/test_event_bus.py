import asyncio
import pytest
from race_engineer.core.event_bus import EventBus


@pytest.mark.asyncio
async def test_event_bus_publish_subscribe():
    bus = EventBus()
    received_events = []

    async def mock_handler(data):
        received_events.append(data)

    bus.subscribe("test_event", mock_handler)

    await bus.publish("test_event", {"key": "value"})
    # wait a tiny bit to let the asyncio.create_task run
    await asyncio.sleep(0.01)

    assert len(received_events) == 1
    assert received_events[0] == {"key": "value"}


@pytest.mark.asyncio
async def test_event_bus_multiple_subscribers():
    bus = EventBus()
    counter = {"count": 0}

    async def async_handler(data):
        counter["count"] += data["inc"]

    def sync_handler(data):
        counter["count"] += data["inc"]

    bus.subscribe("inc_event", async_handler)
    bus.subscribe("inc_event", sync_handler)

    await bus.publish("inc_event", {"inc": 2})
    await asyncio.sleep(0.01)

    assert counter["count"] == 4
