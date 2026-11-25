"""A05 主练习：将文本来源转换为增量事件，并管理来源生命周期。"""

from collections.abc import Iterator
from contextlib import closing


def stream_events(chunks) -> Iterator[dict[str, str]]:
    """chunks 约定为产出 str 的生成器对象，支持 close()。

    本函数接管 chunks 的关闭责任。
    空字符串跳过；其他字符串保持原样，逐项产出 delta 事件。
    正常耗尽并关闭来源后，产出一次 done 事件。
    来源异常向上传播，不产出 done；消费者提前关闭时也应关闭来源。
    必须按需消费，不整体物化输入。
    """
    with closing(chunks) as chunks:
        for chunk in chunks:
            if chunk:
                yield {"type": "delta", "text": chunk}
        yield {"type": "done"}


def demo_chunks():
    try:
        yield "Py"
        yield ""
        yield "thon"
    finally:
        print("source closed")


if __name__ == "__main__":
    with closing(stream_events(demo_chunks())) as events:
        for event in events:
            print(event)
