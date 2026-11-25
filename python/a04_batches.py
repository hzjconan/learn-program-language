"""A04 主练习：惰性清洗并分批产出文档，由用户实现。"""

from collections.abc import Iterable, Iterator


def batched_docs(docs: Iterable[str], batch_size: int) -> Iterator[list[str]]:
    """去除首尾空白，跳过空文本，按 batch_size 逐批产出列表。

    docs 的元素约定为 str，batch_size 约定为 int。
    batch_size <= 0 时，在首次推进生成器时抛出 ValueError。
    不提前把 docs 整体转为列表；支持只能消费一次的迭代器。
    每批使用独立列表，最后不足一批也产出，不产出空批次。
    """
    if batch_size <= 0:
        raise ValueError("batch_size 必须为正整数")

    batch: list[str] = []
    for doc in docs:
        cleaned = doc.strip()
        if not cleaned:
            continue
        batch.append(cleaned)
        if len(batch) == batch_size:
            yield batch
            batch = []
    if batch:
        yield batch


if __name__ == "__main__":
    source = iter(["  A ", "", " B", "   ", "C  "])
    batches = batched_docs(source, batch_size=2)
    print(next(batches))
    print(list(batches))
