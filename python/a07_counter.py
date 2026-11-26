"""A07 共享状态练习：保护跨 await 的读改写。"""

import asyncio


async def add_one(state, name, lock):
    """保留 sleep(0)，用共享 lock 保护完整读改写，产出正确累计结果。"""
    async with lock:
        before = state["count"]
        print("read", name, before)
        await asyncio.sleep(0)
        state["count"] = before + 1
        print("write", name, state["count"])


async def main():
    state = {"count": 0}
    lock = asyncio.Lock()
    tasks = [
        asyncio.create_task(add_one(state, name, lock))
        for name in ["A", "B"]
    ]
    await asyncio.gather(*tasks)
    print("count", state["count"])


if __name__ == "__main__":
    asyncio.run(main())
