"""A07 有界队列练习：固定消费者，按输入顺序保存成功结果。"""

import asyncio


async def run_queued(
    names: list[str], worker, workers: int, buffer_size: int
) -> list[str]:
    """worker 约定为接收 str、成功返回 str 的 async def 函数。

    workers、buffer_size 约定为 int，必须为正；空输入也校验。
    只创建 workers 个消费者 Task，队列最多缓冲 buffer_size 项。
    队列数据为 (index, name)，结束信号为 (None, None)，每个消费者一份。
    每次成功 get 后都对应一次 task_done，包括结束信号。
    返回结果按输入顺序，不修改 names。
    本段先验收成功路径；异常取消的生命周期在 A08 展开。
    """
    if type(workers) != int or workers <= 0:
        raise ValueError("workers must be a positive int")
    if type(buffer_size) != int or buffer_size <= 0:
        raise ValueError("buffer_size must be a positive int")

    q = asyncio.Queue(maxsize=buffer_size)
    results = [""] * len(names)

    async def produce():
        for i, name in enumerate(names):
            await q.put((i, name))
        for _ in range(workers):
            await q.put((None, None))

    async def consume():
        while True:
            i, name = await q.get()
            try:
                if name is None:
                    break
                results[i] = await worker(name)
            finally:
                q.task_done()

    consumers = [asyncio.create_task(consume()) for _ in range(workers)]
    await produce()
    await q.join()
    await asyncio.gather(*consumers)
    return results


async def demo_worker(name: str) -> str:
    print("start", name)
    await asyncio.sleep(0.01 if name == "A" else 0)
    print("end", name)
    return name.lower()


if __name__ == "__main__":
    print(asyncio.run(run_queued(["A", "B", "C", "D"], demo_worker, 2, 1)))
