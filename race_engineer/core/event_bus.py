import asyncio
from typing import Callable, Dict, List, Any

class EventBus:
    """
    A simple asynchronous Event Bus to orchestrate communication between the
    Telemetry Parser, Feedback Engine, and Voice Engine.
    """
    def __init__(self):
        self._subscribers: Dict[str, List[Callable[..., Any]]] = {}

    def subscribe(self, event_type: str, callback: Callable[..., Any]):
        """Subscribe a callback to a specific event type."""
        if event_type not in self._subscribers:
            self._subscribers[event_type] = []
        self._subscribers[event_type].append(callback)

    async def publish(self, event_type: str, *args, **kwargs):
        """Publish an event to all subscribers asynchronously."""
        if event_type in self._subscribers:
            tasks = []
            for callback in self._subscribers[event_type]:
                # If callback is a coroutine, await it, otherwise run it
                if asyncio.iscoroutinefunction(callback):
                    tasks.append(asyncio.create_task(callback(*args, **kwargs)))
                else:
                    # Run synchronous callbacks in a thread pool to avoid blocking
                    loop = asyncio.get_running_loop()
                    tasks.append(loop.run_in_executor(None, callback, *args))
            
            if tasks:
                await asyncio.gather(*tasks)

# Global event bus instance for the application
bus = EventBus()
