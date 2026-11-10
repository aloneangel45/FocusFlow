"""Zero-drift monotonic deadline timer and non-pasteable Friction Challenge generator."""

from __future__ import annotations

import math
import secrets
import time
from typing import Callable

# Unambiguous uppercase alphanumeric characters (excludes 0, O, 1, I)
FRICTION_ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"


def generate_friction_token(length: int = 16) -> str:
    """Generate a cryptographically random high-friction challenge string."""
    if length < 8:
        length = 8
    return "".join(secrets.choice(FRICTION_ALPHABET) for _ in range(length))


class MonotonicPhaseTimer:
    """Drift-free phase countdown timer powered by `time.monotonic()`.

    Holds safely at expiration (`awaiting_transition=True`) so modal dialogs
    never burn break or work time in the background before the user acknowledges them.
    """

    def __init__(self, clock_fn: Callable[[], float] = time.monotonic) -> None:
        self._clock = clock_fn
        self.is_running: bool = False
        self.is_paused: bool = False
        self.awaiting_transition: bool = False
        self.phase: str = "WORK"
        self.total_seconds: float = 0.0
        self._started_monotonic: float = 0.0
        self._deadline: float = 0.0
        self._paused_remaining: float = 0.0

    def start(self, duration_seconds: float, phase: str = "WORK") -> None:
        dur = max(0.0, float(duration_seconds))
        now = self._clock()
        self.phase = phase
        self.total_seconds = dur
        self._started_monotonic = now
        self._deadline = now + dur
        self._paused_remaining = dur
        self.is_running = True
        self.is_paused = False
        self.awaiting_transition = False

    def stop(self) -> None:
        self.is_running = False
        self.is_paused = False
        self.awaiting_transition = False
        self._deadline = 0.0
        self._paused_remaining = 0.0
        self.total_seconds = 0.0

    def pause(self) -> None:
        if not self.is_running or self.is_paused or self.awaiting_transition:
            return
        self._paused_remaining = max(0.0, self._deadline - self._clock())
        self.is_paused = True

    def resume(self) -> None:
        if not self.is_running or not self.is_paused or self.awaiting_transition:
            return
        self._deadline = self._clock() + self._paused_remaining
        self.is_paused = False

    def remaining_seconds_float(self) -> float:
        if not self.is_running or self.awaiting_transition:
            return 0.0
        if self.is_paused:
            return max(0.0, self._paused_remaining)
        return max(0.0, self._deadline - self._clock())

    def remaining_seconds(self) -> int:
        rem = self.remaining_seconds_float()
        return int(math.ceil(rem)) if rem > 0 else 0

    def elapsed_seconds(self) -> int:
        if self.total_seconds <= 0:
            return 0
        rem = self.remaining_seconds_float()
        return max(0, int(round(self.total_seconds - rem)))

    def progress_ratio(self) -> float:
        if self.total_seconds <= 0:
            return 0.0
        return max(0.0, min(1.0, self.remaining_seconds_float() / self.total_seconds))

    def poll_expired(self) -> bool:
        """Return True exactly once when the active phase reaches its deadline.

        Transitions into `awaiting_transition=True` so repeated ticks while a modal
        dialog is open never queue duplicate expiration callbacks or burn the next phase.
        """
        if not self.is_running or self.is_paused or self.awaiting_transition:
            return False
        if self._clock() >= self._deadline:
            self.awaiting_transition = True
            return True
        return False
