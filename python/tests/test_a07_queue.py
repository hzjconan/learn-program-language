import asyncio
from a07_queue import run_queued
import pytest

@pytest.mark.parametrize("names, workers, buffer_size", [
    (["A", "B", "C", "D"], 2, 1),
    (["A"], 3, 1),
    ([], 2, 1),
])
def test_run_queued_valid(names, workers, buffer_size):
    copied_names = names.copy()

    async def worker(name: str) -> str:
        await asyncio.sleep(0)
        return name.lower()

    result = asyncio.run(run_queued(names, worker, workers, buffer_size))
    assert result == [n.lower() for n in names]
    assert copied_names == names

@pytest.mark.parametrize("names, workers, buffer_size", [
    (["A", "B", "C", "D"], 0, 1),
    (["A", "B", "C", "D"], -1, 1),
    (["A", "B", "C", "D"], 1, 0),
    (["A", "B", "C", "D"], 1, -1),
    ([], 0, 1),
    ([], -1, 1),
    ([], 1, 0),
    ([], 1, -1),
])
def test_run_queued_invalid(names, workers, buffer_size):
    async def worker(name: str) -> str:
        raise AssertionError("非法参数时不应执行 worker")
    with pytest.raises(ValueError):
        asyncio.run(run_queued(names, worker, workers, buffer_size))
