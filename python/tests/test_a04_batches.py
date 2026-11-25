"""教师独立验证：检查需求边界、消费时机与跨批次对象隔离。"""

import pytest

from a04_batches import batched_docs


@pytest.mark.parametrize(
    "docs, batch_size, expected",
    [
        ([], 2, []),
        (["", " \t\n"], 2, []),
        ([" A ", "", " B", "   ", "C "], 2, [["A", "B"], ["C"]]),
        (["A", "B", "C", "D"], 2, [["A", "B"], ["C", "D"]]),
        (["A", " B "], 1, [["A"], ["B"]]),
        ([" A ", "B"], 10, [["A", "B"]]),
    ],
)
def test_batch_contents(docs, batch_size, expected):
    original = docs.copy()
    assert list(batched_docs(docs, batch_size)) == expected
    assert docs == original


@pytest.mark.parametrize("batch_size", [0, -1])
def test_invalid_size_is_checked_on_first_advance(batch_size):
    def source():
        raise AssertionError("非法批大小不应读取输入")
        yield "unreachable"

    batches = batched_docs(source(), batch_size)
    with pytest.raises(ValueError):
        next(batches)


def test_reads_only_until_requested_batch():
    seen = []

    def source():
        for text in [" A ", "", " B", "   ", "C "]:
            seen.append(text)
            yield text

    batches = batched_docs(source(), 2)
    assert seen == []
    assert next(batches) == ["A", "B"]
    assert seen == [" A ", "", " B"]
    assert list(batches) == [["C"]]
    assert seen == [" A ", "", " B", "   ", "C "]
    assert list(batches) == []


def test_batches_keep_independent_lists():
    batches = batched_docs(iter(["A", "B", "C", "D", "E"]), 2)
    first = next(batches)
    second = next(batches)
    assert first == ["A", "B"]
    assert second == ["C", "D"]
    assert first is not second
    first.append("caller edit")
    tail = list(batches)
    assert tail == [["E"]]
    assert second == ["C", "D"]
    assert first == ["A", "B", "caller edit"]
    assert tail[0] is not first and tail[0] is not second
