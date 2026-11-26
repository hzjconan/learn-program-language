import pytest
from a07_limited import run_limited
import asyncio

def worker_factory():
    active_workers = dict[str, bool]()
    peak_active = 0

    async def worker(name: str) -> str:
        try:
            active_workers[name] = True
            await asyncio.sleep(0)
        finally:
            nonlocal peak_active
            peak_active = max(peak_active, len([w for w in active_workers.values() if w]))
            active_workers[name] = False
        return name.lower()

    def statistics():
        return len([w for w in active_workers.values() if w]), peak_active

    return worker, statistics

@pytest.mark.parametrize("names, limit, ", [
    (["A", "B", "C", "D"], 2),
    (["A", "B", "C", "D"], 1),
])
def test_run_limited_valid(names: list[str], limit: int):
    original = names.copy()
    worker, statistics = worker_factory()
    result = asyncio.run(run_limited(names, worker, limit))
    assert result == [n.lower() for n in names]
    assert statistics()[0] == 0
    assert statistics()[1] == limit
    assert names == original

@pytest.mark.parametrize("names, limit, ", [
    (["A", "B", "C", "D"], 0),
    (["A", "B", "C", "D"], -1),
    ([], 0),
    ([], -1),
])
def test_run_limited_invalid(names: list[str], limit: int):
    original = names.copy()
    worker, statistics = worker_factory()
    with pytest.raises(ValueError):
        asyncio.run(run_limited(names, worker, limit))
    assert statistics() == (0, 0)

def test_run_limited_no_names():
    worker, statistics = worker_factory()
    result = asyncio.run(run_limited([], worker, 1))
    assert result == []
    assert statistics() == (0, 0)
