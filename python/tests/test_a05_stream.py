from contextlib import closing
import pytest
from a05_stream import stream_events


def test_stream_events_closed():
    read: list[str] = []
    state = {"closed": False}

    def source():
        try:
            for chunk in ["Py", "", "thon"]:
                read.append(chunk)
                yield chunk
        finally:
            state["closed"] = True

    with closing(stream_events(source())) as events:
        assert next(events) == {"type": "delta", "text": "Py"}

    assert state["closed"] is True
    assert read == ["Py"]

def test_stream_events_quit():
    read: list[str] = []
    state = {"closed": False}

    def source(quit=True):
        try:
            for chunk in ["Py", "", "thon"]:
                read.append(chunk)
                yield chunk
                if quit:
                    raise RuntimeError("连接中断")
        finally:
            state["closed"] = True

    with closing(stream_events(source())) as events:
        assert next(events) == {"type": "delta", "text": "Py"}
        with pytest.raises(RuntimeError):
            next(events)

    assert state["closed"] is True
    assert read == ["Py"]
