from time import perf_counter
from typing import Final

MILLISECONDS_PER_SECOND: Final = 1000


def elapsed_ms(started: float) -> int:
    """Whole milliseconds since a `perf_counter()` reading, the unit of every `duration_ms` log field."""
    return round((perf_counter() - started) * MILLISECONDS_PER_SECOND)
