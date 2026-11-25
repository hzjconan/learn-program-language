"""A06 主练习：启动异步任务，按输入顺序收集结果。"""

import asyncio


async def run_jobs(names: list[str], worker) -> list[str]:
    """worker 约定为接收 str、成功返回 str 的 async def 函数。

    先为全部输入创建 Task，再等待结果；结果顺序与 names 一致。
    空输入返回 []，不修改 names，不在本函数内调用 asyncio.run。
    本轮假设 worker 正常返回；失败、取消与限流在后续课程展开。
    """
    if len(names) == 0:
        return []
    tasks = [asyncio.create_task(worker(name)) for name in names]
    results = await asyncio.gather(*tasks)
    return results


async def demo_worker(name: str) -> str:
    print("start", name)
    await asyncio.sleep(0.02 if name == "A" else 0)
    print("end", name)
    return name.lower()


async def main():
    results = await run_jobs(["A", "B"], demo_worker)
    print("results", results)


if __name__ == "__main__":
    asyncio.run(main())
