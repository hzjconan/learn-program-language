from a06_jobs import run_jobs
import asyncio


def test_jobs():
    timeline: list[str] = []

    async def worker(name: str) -> str:
        timeline.append(f"start {name}")
        await asyncio.sleep(0)

        #让 A 多让出一次执行机会，验证最后结果a还是在b前面
        if name == "A":
            await asyncio.sleep(1)

        timeline.append(f"end {name}")
        return name.lower()

    names = ["A", "B", "C"]
    original = names.copy()
    results = asyncio.run(run_jobs(names, worker))

    first_end = next(i for i, item in enumerate(timeline) if item.startswith("end"))
    assert timeline[:first_end] == ["start A", "start B", "start C"]
    ends = timeline[first_end:]
    assert ends.index("end B") < ends.index("end A")
    assert ends.index("end C") < ends.index("end A")
    assert results == ["a", "b", "c"]
    assert names == original


def test_empty_input():
    async def worker():
        raise AssertionError("empty worker")
    assert asyncio.run(run_jobs([], worker)) == []
