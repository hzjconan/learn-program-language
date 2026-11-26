"""A07 第一段练习：用共享信号量限制 worker 的并发数量。"""

import asyncio


async def run_limited(names: list[str], worker, limit: int) -> list[str]:
    """worker 约定为接收 str、正常返回 str 的 async def 函数。

    limit 约定为 int；非正数在本协程运行时抛出 ValueError。
    同时进入 worker 的任务不超过 limit，结果按 names 顺序返回。
    空输入返回 []，不修改 names；空输入也须验证 limit。
    本段允许为每项创建 Task；稍后用有界队列限制待处理项缓冲。
    """
    if type(limit) != int or limit <= 0:
        raise ValueError("limit must be a positive int")
    if names is None or len(names) == 0:
        return []

    semaphore = asyncio.Semaphore(limit)

    async def executor(name: str) -> str:
        async with semaphore:
            return await worker(name)

    tasks = []
    for name in names:
        tasks.append(asyncio.create_task(executor(name)))
    return await asyncio.gather(*tasks)


async def demo_worker(name: str) -> str:
    print("start", name)
    await asyncio.sleep(0)
    print("end", name)
    return name.lower()


if __name__ == "__main__":
    print(asyncio.run(run_limited(["A", "B", "C", "D"], demo_worker, 2)))
